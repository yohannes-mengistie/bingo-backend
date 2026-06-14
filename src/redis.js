const Redis = require("ioredis");
const config = require("./config");

const redis = config.redis.url
  ? new Redis(config.redis.url)
  : new Redis({
      host: config.redis.host,
      port: Number(config.redis.port),
      password: config.redis.password || undefined,
      db: config.redis.db,
    });

redis.on("error", (err) => {
  console.error("Redis error:", err.message);
});

module.exports = {
  redis,
};
