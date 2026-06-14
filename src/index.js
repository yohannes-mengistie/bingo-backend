const express = require("express");
const http = require("http");
const config = require("./config");
const { pool } = require("./db");
const { redis } = require("./redis");
const { createGameStateService } = require("./services/gameStateService");
const { createGameUsecase } = require("./usecases/gameUsecase");
const { createWebSocketServer } = require("./websocket/websocketServer");
const routes = require("./routes");

const app = express();

app.use(express.json({ limit: "1mb" }));
app.use(routes);

app.locals.db = pool;
app.locals.redis = redis;
app.locals.gameStateService = createGameStateService(redis);
app.locals.gameUsecase = createGameUsecase({
  pool,
  redisService: app.locals.gameStateService,
});

app.get("/health", (_req, res) => {
  res.json({ status: "ok" });
});

const server = http.createServer(app);

createWebSocketServer({
  server,
  redis,
  gameUsecase: app.locals.gameUsecase,
  gameStateService: app.locals.gameStateService,
});

server.listen(config.server.port, config.server.host, () => {
  console.log(
    `Node API listening on http://${config.server.host}:${config.server.port}`
  );
});
