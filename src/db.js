const { Pool } = require("pg");
const config = require("./config");

const pool = new Pool({
  host: config.database.host,
  port: Number(config.database.port),
  user: config.database.user,
  password: config.database.password,
  database: config.database.database,
  ssl: config.database.sslmode === "disable" ? false : { rejectUnauthorized: false },
});

module.exports = {
  pool,
};
