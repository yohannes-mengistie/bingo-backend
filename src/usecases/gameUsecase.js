const gameRepository = require("../repositories/gameRepository");
const walletRepository = require("../repositories/walletRepository");
const transactionRepository = require("../repositories/transactionRepository");
const userRepository = require("../repositories/userRepository");
const {
  GameState,
  MinPlayers,
  HouseCut,
  CountdownDurationSeconds,
  CountdownTickerIntervalMs,
  MinCardId,
  MaxCardId,
  CardTotalPositions,
  CardGridSize,
  CardCenterValue,
  MaxAvailableGamesLimit,
  DrawIntervalMs,
  WebSocketEvent,
  getBetAmount,
} = require("../constants/game");
const { TransactionType, TransactionStatus } = require("../constants/transactions");
const { generateCard, validateBingo } = require("../bingo/card");
const { drawNextNumber } = require("../bingo/draw");

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function createGameUsecase({ pool, redisService }) {
  async function getAvailableGames(gameType) {
    return gameRepository.findAvailable(pool, gameType, MaxAvailableGamesLimit);
  }

  async function createOrGetGame(gameType) {
    const games = await gameRepository.findAvailable(pool, gameType, 1);
    if (games.length > 0) {
      return games[0];
    }

    const game = {
      id: require("crypto").randomUUID(),
      game_type: gameType,
      state: GameState.Waiting,
      bet_amount: getBetAmount(gameType),
      min_players: MinPlayers,
      player_count: 0,
      prize_pool: 0,
      house_cut: HouseCut,
    };

    await gameRepository.create(pool, game);

    if (redisService) {
      try {
        await redisService.saveGameState(game);
      } catch (err) {
        console.warn("Warning: failed to save game state to Redis:", err.message);
      }
    }

    return gameRepository.findById(pool, game.id);
  }

  async function joinGame(gameId, request) {
    if (request.card_id < MinCardId || request.card_id > MaxCardId) {
      throw new Error(`card ID must be between ${MinCardId} and ${MaxCardId}`);
    }

    const game = await gameRepository.findById(pool, gameId);
    if (!game) {
      throw new Error("game not found");
    }

    if (game.state !== GameState.Waiting && game.state !== GameState.Countdown) {
      throw new Error("game is not accepting new players");
    }

    const existingPlayer = await gameRepository.findPlayer(pool, gameId, request.user_id);
    if (existingPlayer) {
      throw new Error("user is already in this game");
    }

    const client = await pool.connect();
    let player;
    try {
      await client.query("BEGIN");

      const wallet = await walletRepository.lockForUpdate(client, request.user_id);
      if (!wallet) {
        throw new Error("wallet not found");
      }

      if (wallet.balance < Number(game.bet_amount)) {
        throw new Error("insufficient balance");
      }

      await walletRepository.updateBalance(client, request.user_id, -Number(game.bet_amount));

      const gameBetRef = "GAME_BET";
      await transactionRepository.createTransaction(client, {
        user_id: request.user_id,
        type: TransactionType.Withdraw,
        amount: Number(game.bet_amount),
        status: TransactionStatus.Completed,
        reference: gameBetRef,
      });

      player = {
        id: require("crypto").randomUUID(),
        game_id: gameId,
        user_id: request.user_id,
        card_id: request.card_id,
        is_eliminated: false,
      };

      await gameRepository.addPlayer(client, player);

      const updatedGame = {
        ...game,
        player_count: Number(game.player_count) + 1,
        prize_pool: Number(game.prize_pool) + Number(game.bet_amount) * (1 - Number(game.house_cut)),
      };

      await gameRepository.update(client, updatedGame);
      await client.query("COMMIT");

      if (redisService) {
        await redisService.addPlayer(gameId, request.user_id);
        await redisService.addTakenCard(gameId, request.card_id);
        await redisService.saveGameState(updatedGame);
      }

      if (updatedGame.player_count === MinPlayers) {
        startCountdown(gameId).catch(() => {});
      }

      if (redisService) {
        await redisService.publishEvent(gameId, WebSocketEvent.PlayerJoined, {
          user_id: request.user_id,
          card_id: request.card_id,
        });
      }

      return player;
    } catch (err) {
      await client.query("ROLLBACK");
      throw err;
    } finally {
      client.release();
    }
  }

  async function leaveGame(gameId, request) {
    const game = await gameRepository.findById(pool, gameId);
    if (!game) {
      throw new Error("game not found");
    }

    const player = await gameRepository.findPlayer(pool, gameId, request.user_id);
    if (!player) {
      throw new Error("user is not in this game");
    }

    const client = await pool.connect();
    try {
      await client.query("BEGIN");

      const latestGame = await gameRepository.findById(client, gameId);
      if (!latestGame) {
        throw new Error("game not found");
      }

      if (
        latestGame.state === GameState.Finished ||
        latestGame.state === GameState.Closed ||
        latestGame.state === GameState.Cancelled
      ) {
        throw new Error("game is no longer active");
      }

      await gameRepository.removePlayer(client, gameId, request.user_id);

      if (
        latestGame.state === GameState.Waiting ||
        latestGame.state === GameState.Countdown
      ) {
        await walletRepository.lockForUpdate(client, request.user_id);
        await walletRepository.updateBalance(client, request.user_id, Number(latestGame.bet_amount));

        const gameRefundRef = "GAME_REFUND";
        await transactionRepository.createTransaction(client, {
          user_id: request.user_id,
          type: TransactionType.Deposit,
          amount: Number(latestGame.bet_amount),
          status: TransactionStatus.Completed,
          reference: gameRefundRef,
        });

        latestGame.prize_pool = Number(latestGame.prize_pool) -
          Number(latestGame.bet_amount) * (1 - Number(latestGame.house_cut));
      }

      latestGame.player_count = Number(latestGame.player_count) - 1;

      let revertedFromCountdown = false;
      if (
        latestGame.state === GameState.Countdown &&
        latestGame.player_count < MinPlayers
      ) {
        latestGame.state = GameState.Waiting;
        latestGame.countdown_ends = null;
        revertedFromCountdown = true;
      }

      await gameRepository.update(client, latestGame);
      await client.query("COMMIT");

      if (redisService) {
        await redisService.removePlayer(gameId, request.user_id);
      }

      const players = await gameRepository.getPlayers(pool, gameId);
      const otherPlayersHaveCard = players.some(
        (item) => item.card_id === player.card_id
      );
      if (!otherPlayersHaveCard && redisService) {
        await redisService.removeTakenCard(gameId, player.card_id);
      }

      if (revertedFromCountdown && redisService) {
        await redisService.clearCountdown(gameId);
        await redisService.publishEvent(gameId, WebSocketEvent.GameStatus, {
          status: GameState.Waiting,
        });
      }

      if (redisService) {
        await redisService.saveGameState(latestGame);
        await redisService.publishEvent(gameId, WebSocketEvent.PlayerLeft, {
          user_id: request.user_id,
        });
      }

      return true;
    } catch (err) {
      await client.query("ROLLBACK");
      throw err;
    } finally {
      client.release();
    }
  }

  async function startCountdown(gameId) {
    const game = await gameRepository.findById(pool, gameId);
    if (!game || game.state !== GameState.Waiting) {
      return;
    }

    const countdownEnds = new Date(Date.now() + CountdownDurationSeconds * 1000);
    game.state = GameState.Countdown;
    game.countdown_ends = countdownEnds;

    await gameRepository.update(pool, game);

    if (redisService) {
      await redisService.setCountdown(gameId, countdownEnds);
      await redisService.saveGameState(game);
      await redisService.publishEvent(gameId, WebSocketEvent.GameStatus, {
        status: GameState.Countdown,
        secondsLeft: CountdownDurationSeconds,
      });
    }

    for (let i = CountdownDurationSeconds; i > 0; i -= 1) {
      await delay(CountdownTickerIntervalMs);
      const currentGame = await gameRepository.findById(pool, gameId);
      if (!currentGame || currentGame.state !== GameState.Countdown) {
        return;
      }
      if (currentGame.player_count < MinPlayers) {
        return;
      }
      if (redisService) {
        await redisService.publishEvent(gameId, WebSocketEvent.Countdown, {
          secondsLeft: i - 1,
        });
      }
    }

    await startDrawing(gameId);
  }

  async function startDrawing(gameId) {
    const game = await gameRepository.findById(pool, gameId);
    if (!game || game.state !== GameState.Countdown) {
      return;
    }

    game.state = GameState.Drawing;
    game.started_at = new Date();

    await gameRepository.update(pool, game);

    if (redisService) {
      await redisService.saveGameState(game);
      await redisService.publishEvent(gameId, WebSocketEvent.GameStatus, {
        status: GameState.Drawing,
      });
    }

    drawNumbers(gameId).catch(() => {});
  }

  async function drawNumbers(gameId) {
    const interval = setInterval(async () => {
      try {
        const game = await gameRepository.findById(pool, gameId);
        if (!game || game.state !== GameState.Drawing) {
          clearInterval(interval);
          return;
        }

        let drawnNumbers = [];
        if (redisService) {
          drawnNumbers = await redisService.getDrawnNumbers(gameId);
        }

        const numbers = drawnNumbers.map((item) => item.number);
        const { letter, number } = drawNextNumber(numbers);
        if (!number) {
          return;
        }

        const drawnAt = new Date();
        const drawnNumber = { letter, number, drawn_at: drawnAt.toISOString() };

        if (redisService) {
          await redisService.addDrawnNumber(gameId, drawnNumber);
        }
        await gameRepository.saveDrawnNumber(pool, gameId, letter, number);

        if (redisService) {
          await redisService.publishEvent(gameId, WebSocketEvent.NumberDrawn, {
            letter,
            number,
            drawn_at: drawnAt.toISOString(),
          });
        }
      } catch (err) {
        clearInterval(interval);
      }
    }, DrawIntervalMs);
  }

  async function claimBingo(gameId, request) {
    const game = await gameRepository.findById(pool, gameId);
    if (!game) {
      throw new Error("game not found");
    }

    if (game.state !== GameState.Drawing) {
      throw new Error("game is not in drawing phase");
    }

    const player = await gameRepository.findPlayer(pool, gameId, request.user_id);
    if (!player) {
      throw new Error("user is not in this game");
    }

    if (player.is_eliminated) {
      throw new Error("player is already eliminated");
    }

    const drawnNumbers = redisService
      ? await redisService.getDrawnNumbers(gameId)
      : [];

    const drawnSet = new Set(drawnNumbers.map((item) => item.number));

    const card = generateCard(player.card_id);
    if (!card) {
      throw new Error("invalid card ID");
    }

    const markedNumbers = [];
    for (const pos of request.marked_numbers) {
      if (pos < 0 || pos >= CardTotalPositions) {
        throw new Error(`invalid position: ${pos} (must be 0-${CardTotalPositions - 1})`);
      }

      const row = Math.floor(pos / CardGridSize);
      const col = pos % CardGridSize;
      const cardNumber = card.numbers[row][col];

      if (cardNumber !== CardCenterValue && !drawnSet.has(cardNumber)) {
        throw new Error(`number ${cardNumber} at position ${pos} was not drawn`);
      }

      markedNumbers.push(cardNumber);
    }

    const isValid = validateBingo(card, markedNumbers);

    const client = await pool.connect();
    try {
      await client.query("BEGIN");

      if (isValid) {
        game.state = GameState.Finished;
        game.winner_id = request.user_id;
        game.finished_at = new Date();

        await walletRepository.lockForUpdate(client, request.user_id);
        await walletRepository.updateBalance(client, request.user_id, Number(game.prize_pool));

        const gamePrizeRef = "GAME_PRIZE";
        await transactionRepository.createTransaction(client, {
          user_id: request.user_id,
          type: TransactionType.Deposit,
          amount: Number(game.prize_pool),
          status: TransactionStatus.Completed,
          reference: gamePrizeRef,
        });

        await gameRepository.update(client, game);
        await client.query("COMMIT");

        if (redisService) {
          await redisService.saveGameState(game);
        }

        let winnerName = "Unknown";
        const winner = await userRepository.findById(pool, request.user_id);
        if (winner) {
          winnerName = winner.last_name
            ? `${winner.first_name} ${winner.last_name}`
            : winner.first_name;
        }

        if (redisService) {
          await redisService.publishEvent(gameId, WebSocketEvent.Winner, {
            user_id: request.user_id,
            winner_name: winnerName,
            prize: Number(game.prize_pool),
            card_id: player.card_id,
            marked_numbers: markedNumbers,
          });

          await redisService.publishEvent(gameId, WebSocketEvent.GameStatus, {
            status: GameState.Finished,
          });
        }

        createOrGetGame(game.game_type)
          .then((newGame) => {
            if (redisService && newGame) {
              redisService.publishEvent(gameId, WebSocketEvent.NewGameAvailable, {
                gameId: newGame.id,
                gameType: newGame.game_type,
              });
            }
          })
          .catch(() => {});

        return true;
      }

      await gameRepository.eliminatePlayer(client, gameId, request.user_id);

      const players = await gameRepository.getPlayers(client, gameId);
      const activePlayers = players.filter((p) => !p.is_eliminated);

      if (activePlayers.length === 0) {
        game.state = GameState.Cancelled;
        const gameRefundRef = "GAME_REFUND";

        for (const p of players) {
          try {
            await walletRepository.lockForUpdate(client, p.user_id);
            await walletRepository.updateBalance(client, p.user_id, Number(game.bet_amount));
            await transactionRepository.createTransaction(client, {
              user_id: p.user_id,
              type: TransactionType.Deposit,
              amount: Number(game.bet_amount),
              status: TransactionStatus.Completed,
              reference: gameRefundRef,
            });
          } catch (err) {
            continue;
          }
        }
      }

      await gameRepository.update(client, game);
      await client.query("COMMIT");

      if (redisService) {
        await redisService.saveGameState(game);
        await redisService.publishEvent(gameId, WebSocketEvent.PlayerEliminated, {
          userId: request.user_id,
        });
      }

      if (game.state === GameState.Cancelled && redisService) {
        await redisService.publishEvent(gameId, WebSocketEvent.GameStatus, {
          status: GameState.Cancelled,
        });

        createOrGetGame(game.game_type)
          .then((newGame) => {
            if (redisService && newGame) {
              redisService.publishEvent(gameId, WebSocketEvent.NewGameAvailable, {
                gameId: newGame.id,
                gameType: newGame.game_type,
              });
            }
          })
          .catch(() => {});
      }

      return false;
    } catch (err) {
      await client.query("ROLLBACK");
      throw err;
    } finally {
      client.release();
    }
  }

  async function getGameState(gameId) {
    const game = await gameRepository.findById(pool, gameId);
    if (!game) {
      throw new Error("game not found");
    }

    let drawnNumbers = [];
    let takenCards = [];

    if (redisService) {
      try {
        drawnNumbers = await redisService.getDrawnNumbers(gameId);
        takenCards = await redisService.getTakenCards(gameId);
      } catch (err) {
        takenCards = await gameRepository.getTakenCards(pool, gameId);
      }
    } else {
      takenCards = await gameRepository.getTakenCards(pool, gameId);
    }

    return { game, drawnNumbers, takenCards };
  }

  async function getPlayerInGame(gameId, userId) {
    return gameRepository.findPlayer(pool, gameId, userId);
  }

  async function getCardData(cardId) {
    if (cardId < MinCardId || cardId > MaxCardId) {
      throw new Error(`card ID must be between ${MinCardId} and ${MaxCardId}`);
    }

    return generateCard(cardId);
  }

  async function getGameHistory(userId, limit, offset) {
    const finalLimit = limit > 0 ? limit : 10;
    const finalOffset = offset >= 0 ? offset : 0;
    return gameRepository.findGamesByUserId(pool, userId, finalLimit, finalOffset);
  }

  return {
    getAvailableGames,
    createOrGetGame,
    joinGame,
    leaveGame,
    claimBingo,
    getGameState,
    getPlayerInGame,
    getCardData,
    getGameHistory,
  };
}

module.exports = {
  createGameUsecase,
};
