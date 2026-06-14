const walletRepository = require("../repositories/walletRepository");
const transactionRepository = require("../repositories/transactionRepository");
const {
  TransactionType,
  TransactionStatus,
  MIN_WITHDRAWAL_REMAINING_BALANCE,
} = require("../constants/transactions");

async function approveDeposit(pool, transactionId) {
  const transaction = await transactionRepository.findById(pool, transactionId);
  if (!transaction) {
    throw new Error("transaction not found");
  }

  if (transaction.type !== TransactionType.Deposit) {
    throw new Error("transaction is not a deposit");
  }

  if (transaction.status !== TransactionStatus.Pending) {
    throw new Error(
      `transaction is not pending (current status: ${transaction.status})`
    );
  }

  const client = await pool.connect();
  try {
    await client.query("BEGIN");

    const wallet = await walletRepository.lockForUpdate(
      client,
      transaction.user_id
    );
    if (!wallet) {
      throw new Error("wallet not found");
    }

    await walletRepository.updateBalance(
      client,
      transaction.user_id,
      transaction.amount
    );

    await transactionRepository.updateStatus(
      client,
      transactionId,
      TransactionStatus.Completed
    );

    await client.query("COMMIT");
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }

  return transactionRepository.findById(pool, transactionId);
}

async function rejectDeposit(pool, transactionId) {
  const transaction = await transactionRepository.findById(pool, transactionId);
  if (!transaction) {
    throw new Error("transaction not found");
  }

  if (transaction.type !== TransactionType.Deposit) {
    throw new Error("transaction is not a deposit");
  }

  if (transaction.status !== TransactionStatus.Pending) {
    throw new Error(
      `transaction is not pending (current status: ${transaction.status})`
    );
  }

  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await transactionRepository.updateStatus(
      client,
      transactionId,
      TransactionStatus.Failed
    );
    await client.query("COMMIT");
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }

  return transactionRepository.findById(pool, transactionId);
}

async function approveWithdrawal(pool, transactionId) {
  const transaction = await transactionRepository.findById(pool, transactionId);
  if (!transaction) {
    throw new Error("transaction not found");
  }

  if (transaction.type !== TransactionType.Withdraw) {
    throw new Error("transaction is not a withdrawal");
  }

  if (transaction.status !== TransactionStatus.Pending) {
    throw new Error(
      `transaction is not pending (current status: ${transaction.status})`
    );
  }

  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await transactionRepository.updateStatus(
      client,
      transactionId,
      TransactionStatus.Completed
    );
    await client.query("COMMIT");
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }

  return transactionRepository.findById(pool, transactionId);
}

async function rejectWithdrawal(pool, transactionId) {
  const transaction = await transactionRepository.findById(pool, transactionId);
  if (!transaction) {
    throw new Error("transaction not found");
  }

  if (transaction.type !== TransactionType.Withdraw) {
    throw new Error("transaction is not a withdrawal");
  }

  if (transaction.status !== TransactionStatus.Pending) {
    throw new Error(
      `transaction is not pending (current status: ${transaction.status})`
    );
  }

  const client = await pool.connect();
  try {
    await client.query("BEGIN");

    const wallet = await walletRepository.lockForUpdate(
      client,
      transaction.user_id
    );
    if (!wallet) {
      throw new Error("wallet not found");
    }

    await walletRepository.updateBalance(
      client,
      transaction.user_id,
      transaction.amount
    );

    await transactionRepository.updateStatus(
      client,
      transactionId,
      TransactionStatus.Failed
    );

    await client.query("COMMIT");
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }

  return transactionRepository.findById(pool, transactionId);
}

async function cancelTransaction(pool, transactionId) {
  const transaction = await transactionRepository.findById(pool, transactionId);
  if (!transaction) {
    throw new Error("transaction not found");
  }

  if (transaction.status !== TransactionStatus.Pending) {
    throw new Error(
      `transaction is not pending (current status: ${transaction.status})`
    );
  }

  const client = await pool.connect();
  try {
    await client.query("BEGIN");

    if (transaction.type === TransactionType.Withdraw) {
      const wallet = await walletRepository.lockForUpdate(
        client,
        transaction.user_id
      );
      if (!wallet) {
        throw new Error("wallet not found");
      }

      await walletRepository.updateBalance(
        client,
        transaction.user_id,
        transaction.amount
      );
    }

    await transactionRepository.updateStatus(
      client,
      transactionId,
      TransactionStatus.Cancelled
    );

    await client.query("COMMIT");
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }

  return transactionRepository.findById(pool, transactionId);
}

async function processWithdrawal(pool, userId, amount, accountNumber, accountType) {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");

    const wallet = await walletRepository.lockForUpdate(client, userId);
    if (!wallet) {
      throw new Error("wallet not found");
    }

    const depositCountResult = await client.query(
      `
      SELECT COUNT(*) AS count
      FROM transactions
      WHERE user_id = $1 AND type = $2 AND status = $3
      `,
      [userId, TransactionType.Deposit, TransactionStatus.Completed]
    );

    const depositCount = Number(depositCountResult.rows[0].count || 0);
    if (depositCount === 0) {
      throw new Error(
        "withdrawal not allowed: user must have at least one completed deposit"
      );
    }

    if (wallet.balance < amount) {
      throw new Error("insufficient balance");
    }

    const remainingBalance = wallet.balance - amount;
    if (remainingBalance < MIN_WITHDRAWAL_REMAINING_BALANCE) {
      throw new Error(
        "withdrawal not allowed: remaining balance must be at least 10"
      );
    }

    await walletRepository.updateBalance(client, userId, -amount);

    const transaction = await transactionRepository.createTransaction(client, {
      user_id: userId,
      type: TransactionType.Withdraw,
      amount,
      status: TransactionStatus.Pending,
      transaction_type: accountType,
      transaction_id: accountNumber,
    });

    await client.query("COMMIT");
    return transaction;
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }
}

async function processTransfer(pool, senderId, receiverId, amount) {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");

    const senderWallet = await walletRepository.lockForUpdate(client, senderId);
    if (!senderWallet) {
      throw new Error("sender wallet not found");
    }

    const receiverWallet = await walletRepository.lockForUpdate(
      client,
      receiverId
    );
    if (!receiverWallet) {
      throw new Error("receiver wallet not found");
    }

    if (senderWallet.balance < amount) {
      throw new Error("insufficient balance");
    }

    await walletRepository.updateBalance(client, senderId, -amount);
    await walletRepository.updateBalance(client, receiverId, amount);

    const senderTransaction = await transactionRepository.createTransaction(
      client,
      {
        user_id: senderId,
        type: TransactionType.TransferOut,
        amount,
        status: TransactionStatus.Completed,
        reference: receiverId,
      }
    );

    const receiverTransaction = await transactionRepository.createTransaction(
      client,
      {
        user_id: receiverId,
        type: TransactionType.TransferIn,
        amount,
        status: TransactionStatus.Completed,
        reference: senderId,
      }
    );

    await client.query("COMMIT");
    return { senderTransaction, receiverTransaction };
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }
}

module.exports = {
  approveDeposit,
  rejectDeposit,
  approveWithdrawal,
  rejectWithdrawal,
  cancelTransaction,
  processWithdrawal,
  processTransfer,
};
