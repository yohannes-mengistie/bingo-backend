const { randomUUID } = require("crypto");

async function createTransaction(db, transaction) {
  const now = new Date();
  const id = transaction.id || randomUUID();

  const query = `
    INSERT INTO transactions (id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
  `;

  const values = [
    id,
    transaction.user_id,
    transaction.type,
    transaction.amount,
    transaction.status,
    transaction.transaction_type || null,
    transaction.transaction_id || null,
    transaction.reference || null,
    now,
  ];

  await db.query(query, values);

  return {
    id,
    user_id: transaction.user_id,
    type: transaction.type,
    amount: Number(transaction.amount),
    status: transaction.status,
    transaction_type: transaction.transaction_type || null,
    transaction_id: transaction.transaction_id || null,
    reference: transaction.reference || null,
    created_at: now,
  };
}

async function findById(db, id) {
  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    WHERE id = $1
  `;

  const result = await db.query(query, [id]);
  if (result.rowCount === 0) {
    return null;
  }

  return mapTransactionRow(result.rows[0]);
}

async function findByUserId(db, userId, limit, offset) {
  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    WHERE user_id = $1
    ORDER BY created_at DESC
    LIMIT $2 OFFSET $3
  `;

  const result = await db.query(query, [userId, limit, offset]);
  return result.rows.map(mapTransactionRow);
}

async function findByUserIdAndType(db, userId, transactionType, limit) {
  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    WHERE user_id = $1
      AND type = $2
      AND (reference IS NULL OR reference NOT LIKE 'GAME_%')
    ORDER BY created_at DESC
    LIMIT $3
  `;

  const result = await db.query(query, [userId, transactionType, limit]);
  return result.rows.map(mapTransactionRow);
}

async function findByUserIdAndTypes(db, userId, transactionTypes, limit) {
  if (!transactionTypes || transactionTypes.length === 0) {
    return [];
  }

  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    WHERE user_id = $1
      AND type = ANY($2)
      AND (reference IS NULL OR reference NOT LIKE 'GAME_%')
    ORDER BY created_at DESC
    LIMIT $3
  `;

  const result = await db.query(query, [userId, transactionTypes, limit]);
  return result.rows.map(mapTransactionRow);
}

async function findByStatusAndType(db, status, transactionType, limit, offset) {
  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    WHERE status = $1 AND type = $2
    ORDER BY created_at DESC
    LIMIT $3 OFFSET $4
  `;

  const result = await db.query(query, [status, transactionType, limit, offset]);
  return result.rows.map(mapTransactionRow);
}

async function findByStatus(db, status, limit, offset) {
  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    WHERE status = $1
    ORDER BY created_at DESC
    LIMIT $2 OFFSET $3
  `;

  const result = await db.query(query, [status, limit, offset]);
  return result.rows.map(mapTransactionRow);
}

async function findByTypes(db, transactionTypes, limit, offset) {
  if (!transactionTypes || transactionTypes.length === 0) {
    return [];
  }

  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    WHERE type = ANY($1)
    ORDER BY created_at DESC
    LIMIT $2 OFFSET $3
  `;

  const result = await db.query(query, [transactionTypes, limit, offset]);
  return result.rows.map(mapTransactionRow);
}

async function findAll(db, limit, offset) {
  const query = `
    SELECT id, user_id, type, amount, status, transaction_type, transaction_id, reference, created_at
    FROM transactions
    ORDER BY created_at DESC
    LIMIT $1 OFFSET $2
  `;

  const result = await db.query(query, [limit, offset]);
  return result.rows.map(mapTransactionRow);
}

async function updateStatus(db, id, status) {
  const query = `
    UPDATE transactions
    SET status = $2
    WHERE id = $1
  `;

  const result = await db.query(query, [id, status]);
  if (result.rowCount === 0) {
    const error = new Error("transaction not found");
    error.code = "NOT_FOUND";
    throw error;
  }
}

async function countByStatusAndType(db, status, transactionType) {
  const query = `
    SELECT COUNT(*) AS count
    FROM transactions
    WHERE status = $1 AND type = $2
  `;

  const result = await db.query(query, [status, transactionType]);
  return Number(result.rows[0].count || 0);
}

async function countAll(db) {
  const result = await db.query("SELECT COUNT(*) AS count FROM transactions");
  return Number(result.rows[0].count || 0);
}

function mapTransactionRow(row) {
  return {
    id: row.id,
    user_id: row.user_id,
    type: row.type,
    amount: Number(row.amount),
    status: row.status,
    transaction_type: row.transaction_type || null,
    transaction_id: row.transaction_id || null,
    reference: row.reference || null,
    created_at: row.created_at,
  };
}

module.exports = {
  createTransaction,
  findById,
  findByUserId,
  findByUserIdAndType,
  findByUserIdAndTypes,
  findByStatusAndType,
  findByStatus,
  findByTypes,
  findAll,
  updateStatus,
  countByStatusAndType,
  countAll,
};
