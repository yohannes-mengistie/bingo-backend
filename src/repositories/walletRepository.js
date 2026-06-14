async function createWallet(client, wallet) {
  const now = new Date();
  const query = `
    INSERT INTO wallets (user_id, balance, demo_balance, updated_at)
    VALUES ($1, $2, $3, $4)
  `;

  const values = [wallet.userId, wallet.balance, wallet.demoBalance, now];
  await client.query(query, values);

  return {
    user_id: wallet.userId,
    balance: wallet.balance,
    demo_balance: wallet.demoBalance,
    updated_at: now,
  };
}

async function findByUserId(pool, userId) {
  const query = `
    SELECT user_id, balance, demo_balance, updated_at
    FROM wallets
    WHERE user_id = $1
  `;

  const result = await pool.query(query, [userId]);
  if (result.rowCount === 0) {
    return null;
  }

  const row = result.rows[0];
  return {
    user_id: row.user_id,
    balance: Number(row.balance),
    demo_balance: Number(row.demo_balance),
    updated_at: row.updated_at,
  };
}

async function lockForUpdate(client, userId) {
  const query = `
    SELECT user_id, balance, demo_balance, updated_at
    FROM wallets
    WHERE user_id = $1
    FOR UPDATE
  `;

  const result = await client.query(query, [userId]);
  if (result.rowCount === 0) {
    return null;
  }

  const row = result.rows[0];
  return {
    user_id: row.user_id,
    balance: Number(row.balance),
    demo_balance: Number(row.demo_balance),
    updated_at: row.updated_at,
  };
}

async function updateBalance(client, userId, amount) {
  const query = `
    UPDATE wallets
    SET balance = balance + $2, updated_at = NOW()
    WHERE user_id = $1
  `;

  const result = await client.query(query, [userId, amount]);
  if (result.rowCount === 0) {
    const error = new Error("wallet not found");
    error.code = "NOT_FOUND";
    throw error;
  }
}

async function updateWallet(pool, wallet) {
  const query = `
    UPDATE wallets
    SET balance = $2, demo_balance = $3, updated_at = NOW()
    WHERE user_id = $1
  `;

  const result = await pool.query(query, [
    wallet.user_id,
    wallet.balance,
    wallet.demo_balance,
  ]);
  if (result.rowCount === 0) {
    const error = new Error("wallet not found");
    error.code = "NOT_FOUND";
    throw error;
  }
}

async function getTotalBalance(pool) {
  const result = await pool.query("SELECT COALESCE(SUM(balance), 0) AS total FROM wallets");
  return Number(result.rows[0].total || 0);
}

module.exports = {
  createWallet,
  findByUserId,
  lockForUpdate,
  updateBalance,
  updateWallet,
  getTotalBalance,
};
