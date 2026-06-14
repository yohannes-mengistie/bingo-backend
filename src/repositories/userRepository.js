const { randomUUID } = require("crypto");

async function createUser(client, user) {
  const now = new Date();
  const id = user.id || randomUUID();
  const role = user.role || "user";

  const query = `
    INSERT INTO users (id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
  `;

  const values = [
    id,
    user.telegramId,
    user.firstName,
    user.lastName || null,
    user.phoneNumber,
    user.referralCode,
    role,
    user.password || null,
    now,
    now,
  ];

  await client.query(query, values);

  return {
    id,
    telegram_id: user.telegramId,
    first_name: user.firstName,
    last_name: user.lastName || null,
    phone_number: user.phoneNumber,
    referal_code: user.referralCode,
    role,
    created_at: now,
    updated_at: now,
  };
}

async function findByTelegramId(pool, telegramId) {
  const query = `
    SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
    FROM users
    WHERE telegram_id = $1
  `;

  const result = await pool.query(query, [telegramId]);
  if (result.rowCount === 0) {
    return null;
  }

  return mapUserRow(result.rows[0]);
}

async function findByPhone(pool, phoneNumber) {
  const query = `
    SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
    FROM users
    WHERE phone_number = $1
  `;

  const result = await pool.query(query, [phoneNumber]);
  if (result.rowCount === 0) {
    return null;
  }

  return mapUserRow(result.rows[0]);
}

async function findByReferralCode(pool, referralCode) {
  const query = `
    SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
    FROM users
    WHERE referal_code = $1
  `;

  const result = await pool.query(query, [referralCode]);
  if (result.rowCount === 0) {
    return null;
  }

  return mapUserRow(result.rows[0]);
}

async function findById(pool, userId) {
  const query = `
    SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
    FROM users
    WHERE id = $1
  `;

  const result = await pool.query(query, [userId]);
  if (result.rowCount === 0) {
    return null;
  }

  return mapUserRow(result.rows[0]);
}

async function findAll(pool, limit, offset) {
  const query = `
    SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
    FROM users
    ORDER BY created_at DESC
    LIMIT $1 OFFSET $2
  `;

  const result = await pool.query(query, [limit, offset]);
  return result.rows.map(mapUserRow).map((user) => ({
    ...user,
    password: null,
  }));
}

async function update(pool, userId, updates) {
  const query = `
    UPDATE users
    SET first_name = $2, last_name = $3, phone_number = $4, updated_at = NOW()
    WHERE id = $1
    RETURNING id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
  `;

  const result = await pool.query(query, [
    userId,
    updates.first_name,
    updates.last_name,
    updates.phone_number,
  ]);

  if (result.rowCount === 0) {
    const error = new Error("user not found");
    error.code = "NOT_FOUND";
    throw error;
  }

  const user = mapUserRow(result.rows[0]);
  user.password = null;
  return user;
}

async function countAll(pool) {
  const result = await pool.query("SELECT COUNT(*) AS count FROM users");
  return Number(result.rows[0].count || 0);
}

async function setAdminCredentialsByTelegramId(pool, telegramId, hashedPassword) {
  const query = `
    UPDATE users
    SET role = 'admin', password = $2, updated_at = NOW()
    WHERE telegram_id = $1
  `;

  const result = await pool.query(query, [telegramId, hashedPassword]);
  if (result.rowCount === 0) {
    const error = new Error("user not found");
    error.code = "NOT_FOUND";
    throw error;
  }
}

function mapUserRow(row) {
  return {
    id: row.id,
    telegram_id: Number(row.telegram_id),
    first_name: row.first_name,
    last_name: row.last_name,
    phone_number: row.phone_number,
    referal_code: row.referal_code,
    role: row.role,
    password: row.password,
    created_at: row.created_at,
    updated_at: row.updated_at,
  };
}

module.exports = {
  createUser,
  findByTelegramId,
  findByPhone,
  findByReferralCode,
  findById,
  findAll,
  update,
  countAll,
  setAdminCredentialsByTelegramId,
};
