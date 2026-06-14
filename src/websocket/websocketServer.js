const WebSocket = require("ws");
const { URL } = require("url");
const {
  GameType,
  GameState,
  WebSocketEvent,
  WebSocketInitialStateTimeoutMs,
} = require("../constants/game");

function isValidGameType(gameType) {
  return Object.values(GameType).includes(gameType);
}

function parseGameIdFromPath(pathname) {
  const parts = pathname.split("/").filter(Boolean);
  if (parts.length < 5) {
    return null;
  }
  return parts[4] || null;
}

function createWebSocketServer({ server, redis, gameUsecase, gameStateService }) {
  const wss = new WebSocket.Server({ noServer: true });

  server.on("upgrade", (req, socket, head) => {
    try {
      const url = new URL(req.url, "http://localhost");
      const pathname = url.pathname || "";
      if (
        pathname !== "/api/v1/ws/game" &&
        !pathname.startsWith("/api/v1/ws/game/")
      ) {
        socket.destroy();
        return;
      }

      wss.handleUpgrade(req, socket, head, (ws) => {
        wss.emit("connection", ws, req);
      });
    } catch (_err) {
      socket.destroy();
    }
  });

  wss.on("connection", (ws, req) => {
    handleConnection(ws, req, { redis, gameUsecase, gameStateService });
  });

  return wss;
}

async function handleConnection(ws, req, { redis, gameUsecase, gameStateService }) {
  const url = new URL(req.url, "http://localhost");
  const pathname = url.pathname || "";
  const gameType = url.searchParams.get("type");
  const gameIdParam = pathname.startsWith("/api/v1/ws/game/")
    ? parseGameIdFromPath(pathname)
    : null;

  let gameId = null;
  let game = null;

  if (gameType) {
    if (!isValidGameType(gameType)) {
      ws.close(1008, "Invalid game type");
      return;
    }

    if (!gameUsecase) {
      ws.close(1011, "Game service not available");
      return;
    }

    try {
      game = await gameUsecase.createOrGetGame(gameType);
      gameId = game.id;
    } catch (err) {
      ws.close(1011, "Failed to create or get game");
      return;
    }
  } else if (gameIdParam) {
    gameId = gameIdParam;
  } else {
    ws.close(1008, "No game type or game ID provided");
    return;
  }

  if (!redis) {
    ws.close(1011, "Redis not configured");
    return;
  }

  try {
    await redis.ping();
  } catch (_err) {
    ws.close(1011, "Redis unavailable");
    return;
  }

  const subscriber = redis.duplicate();
  const channel = `game:${gameId}:events`;

  let isClosed = false;
  const closeConnection = async () => {
    if (isClosed) {
      return;
    }
    isClosed = true;
    try {
      await subscriber.quit();
    } catch (_err) {
      // ignore
    }
  };

  ws.on("close", closeConnection);
  ws.on("error", closeConnection);

  subscriber.on("message", (_channel, message) => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.send(message);
    }
  });

  try {
    await subscriber.subscribe(channel);
  } catch (_err) {
    ws.close(1011, "Subscription failed");
    await closeConnection();
    return;
  }

  await sendInitialState(ws, gameId, gameUsecase, gameStateService);

  const pingInterval = setInterval(() => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.ping();
    }
  }, 54000);

  ws.on("close", () => {
    clearInterval(pingInterval);
  });
}

async function sendInitialState(ws, gameId, gameUsecase, gameStateService) {
  let game = null;
  let drawnNumbers = [];
  let takenCards = [];
  let playerCount = 0;
  let secondsLeft = 0;

  const deadline = Date.now() + WebSocketInitialStateTimeoutMs;

  if (gameStateService) {
    try {
      game = await gameStateService.getGameState(gameId);
    } catch (_err) {
      game = null;
    }
  }

  if (!game && gameUsecase) {
    try {
      const result = await gameUsecase.getGameState(gameId);
      game = result.game;
      drawnNumbers = result.drawnNumbers || [];
      takenCards = result.takenCards || [];
      playerCount = game ? Number(game.player_count || 0) : 0;
    } catch (_err) {
      game = null;
    }
  }

  if (gameStateService && game) {
    try {
      drawnNumbers = await gameStateService.getDrawnNumbers(gameId);
      takenCards = await gameStateService.getTakenCards(gameId);
      playerCount = await gameStateService.getPlayerCount(gameId);
    } catch (_err) {
      // ignore
    }
  }

  if (game && game.state === GameState.Countdown) {
    if (gameStateService) {
      try {
        const countdownEnds = await gameStateService.getCountdown(gameId);
        secondsLeft = Math.max(0, Math.floor((countdownEnds.getTime() - Date.now()) / 1000));
      } catch (_err) {
        secondsLeft = 0;
      }
    } else if (game.countdown_ends) {
      const countdownEnds = new Date(game.countdown_ends);
      secondsLeft = Math.max(0, Math.floor((countdownEnds.getTime() - Date.now()) / 1000));
    }
  }

  const payload = {
    event: WebSocketEvent.InitialState,
    data: {
      game: game || null,
      drawnNumbers: drawnNumbers || [],
      takenCards: takenCards || [],
      playerCount: playerCount || 0,
      secondsLeft: secondsLeft || 0,
    },
  };

  if (Date.now() > deadline) {
    return;
  }

  if (ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(payload));
  }
}

module.exports = {
  createWebSocketServer,
};
