const dotenv = require("dotenv");

dotenv.config();

const config = {
  server: {
    port: process.env.PORT || process.env.SERVER_PORT || "8000",
    host: process.env.SERVER_HOST || "0.0.0.0",
  },
  database: {
    host: process.env.DB_HOST || process.env.PGHOST || "localhost",
    port: process.env.DB_PORT || process.env.PGPORT || "5432",
    user: process.env.DB_USER || process.env.PGUSER || "postgres",
    password: process.env.DB_PASSWORD || process.env.PGPASSWORD || "postgres",
    database: process.env.DB_NAME || process.env.PGDATABASE || "bingo",
    sslmode: process.env.DB_SSLMODE || process.env.PGSSLMODE || "disable",
  },
  redis: {
    host: process.env.REDIS_HOST || "localhost",
    port: process.env.REDIS_PORT || "6379",
    password: process.env.REDIS_PASSWORD || "",
    db: parseInt(process.env.REDIS_DB || "0", 10),
    url: process.env.REDIS_URL || "",
  },
  jwt: {
    secret: process.env.JWT_SECRET || "your-secret-key-change-in-production",
    expirationHours: parseInt(process.env.JWT_EXPIRATION_HOURS || "24", 10),
  },
  adminSecretCode: process.env.SECRET_CODE || "",
};

module.exports = config;
