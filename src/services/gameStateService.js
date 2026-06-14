function gameStateKey(gameId) {
  return `game:${gameId}:state`;
}

function gamePlayersKey(gameId) {
  return `game:${gameId}:players`;
}

function gameDrawnNumbersKey(gameId) {
  return `game:${gameId}:drawn`;
}

function gameTakenCardsKey(gameId) {
  return `game:${gameId}:cards:taken`;
}

function gameCountdownKey(gameId) {
  return `game:${gameId}:countdown`;
}

function gameChannel(gameId) {
  return `game:${gameId}:events`;
}

function createGameStateService(client) {
  function ensureClient() {
    if (!client) {
      throw new Error("Redis client is not configured");
    }
  }

  async function saveGameState(game) {
    ensureClient();
    const key = gameStateKey(game.id || game.ID || game.game_id || game.gameId);
    const data = JSON.stringify(game);
    await client.set(key, data, "EX", 24 * 60 * 60);
  }

  async function getGameState(gameId) {
    ensureClient();
    const key = gameStateKey(gameId);
    const data = await client.get(key);
    if (!data) {
      throw new Error("game state not found");
    }
    return JSON.parse(data);
  }

  async function addPlayer(gameId, userId) {
    ensureClient();
    return client.sadd(gamePlayersKey(gameId), String(userId));
  }

  async function removePlayer(gameId, userId) {
    ensureClient();
    return client.srem(gamePlayersKey(gameId), String(userId));
  }

  async function getPlayerCount(gameId) {
    ensureClient();
    return client.scard(gamePlayersKey(gameId));
  }

  async function isPlayerInGame(gameId, userId) {
    ensureClient();
    const result = await client.sismember(gamePlayersKey(gameId), String(userId));
    return result === 1;
  }

  async function addDrawnNumber(gameId, number) {
    ensureClient();
    const data = JSON.stringify(number);
    return client.rpush(gameDrawnNumbersKey(gameId), data);
  }

  async function getDrawnNumbers(gameId) {
    ensureClient();
    const data = await client.lrange(gameDrawnNumbersKey(gameId), 0, -1);
    return data
      .map((item) => {
        try {
          return JSON.parse(item);
        } catch (err) {
          return null;
        }
      })
      .filter(Boolean);
  }

  async function addTakenCard(gameId, cardId) {
    ensureClient();
    return client.sadd(gameTakenCardsKey(gameId), String(cardId));
  }

  async function removeTakenCard(gameId, cardId) {
    ensureClient();
    return client.srem(gameTakenCardsKey(gameId), String(cardId));
  }

  async function getTakenCards(gameId) {
    ensureClient();
    const members = await client.smembers(gameTakenCardsKey(gameId));
    return members.map((item) => Number.parseInt(item, 10)).filter(Number.isFinite);
  }

  async function setCountdown(gameId, endsAt) {
    ensureClient();
    return client.set(gameCountdownKey(gameId), Math.floor(endsAt.getTime() / 1000), "EX", 120);
  }

  async function getCountdown(gameId) {
    ensureClient();
    const seconds = await client.get(gameCountdownKey(gameId));
    if (!seconds) {
      throw new Error("countdown not found");
    }
    return new Date(Number(seconds) * 1000);
  }

  async function clearCountdown(gameId) {
    ensureClient();
    return client.del(gameCountdownKey(gameId));
  }

  async function publishEvent(gameId, event, data) {
    ensureClient();
    const payload = JSON.stringify({ event, data });
    return client.publish(gameChannel(gameId), payload);
  }

  async function deleteGameState(gameId) {
    ensureClient();
    const keys = [
      gameStateKey(gameId),
      gamePlayersKey(gameId),
      gameDrawnNumbersKey(gameId),
      gameTakenCardsKey(gameId),
      gameCountdownKey(gameId),
    ];
    return client.del(...keys);
  }

  return {
    saveGameState,
    getGameState,
    addPlayer,
    removePlayer,
    getPlayerCount,
    isPlayerInGame,
    addDrawnNumber,
    getDrawnNumbers,
    addTakenCard,
    removeTakenCard,
    getTakenCards,
    setCountdown,
    getCountdown,
    clearCountdown,
    publishEvent,
    deleteGameState,
    keys: {
      gameStateKey,
      gamePlayersKey,
      gameDrawnNumbersKey,
      gameTakenCardsKey,
      gameCountdownKey,
      gameChannel,
    },
  };
}

module.exports = {
  createGameStateService,
};
