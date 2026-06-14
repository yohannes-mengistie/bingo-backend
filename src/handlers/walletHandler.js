const walletUsecase = require("../usecases/walletUsecase");
const { getPaginationParams } = require("../utils/pagination");

const uuidRegex =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function isUuid(value) {
  return typeof value === "string" && uuidRegex.test(value);
}

function isValidDepositBody(body) {
  return (
    body &&
    isUuid(body.user_id) &&
    typeof body.amount === "number" &&
    body.amount > 0 &&
    typeof body.transaction_type === "string" &&
    body.transaction_type.length > 0 &&
    typeof body.transaction_id === "string" &&
    body.transaction_id.length > 0
  );
}

function isValidWithdrawBody(body) {
  return (
    body &&
    isUuid(body.user_id) &&
    typeof body.amount === "number" &&
    body.amount > 0 &&
    typeof body.account_number === "string" &&
    body.account_number.length > 0 &&
    typeof body.account_type === "string" &&
    body.account_type.length > 0
  );
}

function isValidTransferBody(body) {
  return (
    body &&
    isUuid(body.sender_id) &&
    isUuid(body.receiver_id) &&
    typeof body.amount === "number" &&
    body.amount > 0
  );
}

async function deposit(req, res) {
  if (!isValidDepositBody(req.body)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const transaction = await walletUsecase.deposit(
      req.app.locals.db,
      req.body
    );

    return res.status(201).json({
      message: "Deposit request created successfully",
      transaction,
    });
  } catch (err) {
    if (err.message === "amount must be greater than 0") {
      return res.status(400).json({ error: err.message });
    }
    if (err.message === "user not found") {
      return res.status(404).json({ error: err.message });
    }

    return res.status(500).json({ error: err.message });
  }
}

async function withdraw(req, res) {
  if (!isValidWithdrawBody(req.body)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const transaction = await walletUsecase.withdraw(
      req.app.locals.db,
      req.body
    );

    return res.status(200).json({
      message: "Withdrawal processed successfully",
      transaction,
    });
  } catch (err) {
    if (
      err.message === "amount must be greater than 0" ||
      err.message === "account_type must be either CBE or Telebirr" ||
      err.message === "account_number is required"
    ) {
      return res.status(400).json({ error: err.message });
    }

    if (err.message === "user not found" || err.message === "wallet not found") {
      return res.status(404).json({ error: err.message });
    }

    if (
      err.message === "insufficient balance" ||
      err.message ===
        "withdrawal not allowed: remaining balance must be at least 10" ||
      err.message ===
        "withdrawal not allowed: user must have at least one completed deposit"
    ) {
      return res.status(400).json({ error: err.message });
    }

    return res.status(500).json({ error: err.message });
  }
}

async function transfer(req, res) {
  if (!isValidTransferBody(req.body)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const result = await walletUsecase.transfer(
      req.app.locals.db,
      req.body
    );

    return res.status(200).json({
      message: "Transfer completed successfully",
      sender_tx: result.senderTransaction,
      receiver_tx: result.receiverTransaction,
    });
  } catch (err) {
    if (err.message === "amount must be greater than 0") {
      return res.status(400).json({ error: err.message });
    }
    if (err.message === "cannot transfer to yourself") {
      return res.status(400).json({ error: err.message });
    }
    if (
      err.message === "sender not found" ||
      err.message === "receiver not found" ||
      err.message === "sender wallet not found" ||
      err.message === "receiver wallet not found"
    ) {
      return res.status(404).json({ error: err.message });
    }
    if (err.message === "insufficient balance") {
      return res.status(400).json({ error: err.message });
    }

    return res.status(500).json({ error: err.message });
  }
}

async function getWallet(req, res) {
  const userId = req.params.user_id;
  if (!isUuid(userId)) {
    return res.status(400).json({ error: "Invalid user ID" });
  }

  try {
    const wallet = await walletUsecase.getWallet(req.app.locals.db, userId);
    return res.status(200).json({ wallet });
  } catch (err) {
    return res.status(404).json({ error: "Wallet not found" });
  }
}

async function getWalletByTelegramId(req, res) {
  const telegramId = Number.parseInt(req.params.telegram_id, 10);
  if (!Number.isFinite(telegramId)) {
    return res.status(400).json({ error: "Invalid telegram ID" });
  }

  try {
    const wallet = await walletUsecase.getWalletByTelegramId(
      req.app.locals.db,
      telegramId
    );

    return res.status(200).json({ wallet });
  } catch (err) {
    return res.status(404).json({ error: "Wallet not found" });
  }
}

async function getDepositHistory(req, res) {
  const userId = req.params.user_id;
  if (!isUuid(userId)) {
    return res.status(400).json({ error: "Invalid user ID" });
  }

  const limit = req.query.all === "true" ? 10000 : 10;
  try {
    const deposits = await walletUsecase.getDepositHistory(
      req.app.locals.db,
      userId,
      limit
    );
    return res.status(200).json({ deposits, count: deposits.length });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch deposit history" });
  }
}

async function getWithdrawalHistory(req, res) {
  const userId = req.params.user_id;
  if (!isUuid(userId)) {
    return res.status(400).json({ error: "Invalid user ID" });
  }

  const limit = req.query.all === "true" ? 10000 : 10;
  try {
    const withdrawals = await walletUsecase.getWithdrawalHistory(
      req.app.locals.db,
      userId,
      limit
    );
    return res.status(200).json({ withdrawals, count: withdrawals.length });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch withdrawal history" });
  }
}

async function getTransferHistory(req, res) {
  const userId = req.params.user_id;
  if (!isUuid(userId)) {
    return res.status(400).json({ error: "Invalid user ID" });
  }

  const limit = req.query.all === "true" ? 10000 : 10;
  try {
    const transfers = await walletUsecase.getTransferHistory(
      req.app.locals.db,
      userId,
      limit
    );
    return res.status(200).json({ transfers, count: transfers.length });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch transfer history" });
  }
}

async function approveDeposit(req, res) {
  const transactionId = req.params.id;
  if (!isUuid(transactionId)) {
    return res.status(400).json({ error: "Invalid transaction ID" });
  }

  try {
    const transaction = await walletUsecase.approveDeposit(
      req.app.locals.db,
      transactionId
    );
    return res.status(200).json({
      message: "Deposit approved successfully",
      transaction,
    });
  } catch (err) {
    if (err.message === "transaction not found") {
      return res.status(404).json({ error: err.message });
    }
    if (
      err.message === "transaction is not a deposit" ||
      err.message.startsWith("transaction is not pending")
    ) {
      return res.status(400).json({ error: err.message });
    }
    return res.status(500).json({ error: err.message });
  }
}

async function rejectDeposit(req, res) {
  const transactionId = req.params.id;
  if (!isUuid(transactionId)) {
    return res.status(400).json({ error: "Invalid transaction ID" });
  }

  try {
    const transaction = await walletUsecase.rejectDeposit(
      req.app.locals.db,
      transactionId
    );
    return res.status(200).json({
      message: "Deposit rejected successfully",
      transaction,
    });
  } catch (err) {
    if (err.message === "transaction not found") {
      return res.status(404).json({ error: err.message });
    }
    if (
      err.message === "transaction is not a deposit" ||
      err.message.startsWith("transaction is not pending")
    ) {
      return res.status(400).json({ error: err.message });
    }
    return res.status(500).json({ error: err.message });
  }
}

async function approveWithdrawal(req, res) {
  const transactionId = req.params.id;
  if (!isUuid(transactionId)) {
    return res.status(400).json({ error: "Invalid transaction ID" });
  }

  try {
    const transaction = await walletUsecase.approveWithdrawal(
      req.app.locals.db,
      transactionId
    );
    return res.status(200).json({
      message: "Withdrawal approved successfully",
      transaction,
    });
  } catch (err) {
    if (err.message === "transaction not found") {
      return res.status(404).json({ error: err.message });
    }
    if (
      err.message === "transaction is not a withdrawal" ||
      err.message.startsWith("transaction is not pending")
    ) {
      return res.status(400).json({ error: err.message });
    }
    return res.status(500).json({ error: err.message });
  }
}

async function rejectWithdrawal(req, res) {
  const transactionId = req.params.id;
  if (!isUuid(transactionId)) {
    return res.status(400).json({ error: "Invalid transaction ID" });
  }

  try {
    const transaction = await walletUsecase.rejectWithdrawal(
      req.app.locals.db,
      transactionId
    );
    return res.status(200).json({
      message: "Withdrawal rejected and balance refunded",
      transaction,
    });
  } catch (err) {
    if (err.message === "transaction not found") {
      return res.status(404).json({ error: err.message });
    }
    if (
      err.message === "transaction is not a withdrawal" ||
      err.message.startsWith("transaction is not pending")
    ) {
      return res.status(400).json({ error: err.message });
    }
    return res.status(500).json({ error: err.message });
  }
}

async function cancelTransaction(req, res) {
  const transactionId = req.params.id;
  if (!isUuid(transactionId)) {
    return res.status(400).json({ error: "Invalid transaction ID" });
  }

  try {
    const transaction = await walletUsecase.cancelTransaction(
      req.app.locals.db,
      transactionId
    );
    return res.status(200).json({
      message: "Transaction cancelled successfully",
      transaction,
    });
  } catch (err) {
    if (err.message === "transaction not found") {
      return res.status(404).json({ error: err.message });
    }
    if (err.message.startsWith("transaction is not pending")) {
      return res.status(400).json({ error: err.message });
    }
    return res.status(500).json({ error: err.message });
  }
}

async function getPendingDeposits(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const transactions = await walletUsecase.getPendingDeposits(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      transactions,
      count: transactions.length,
      limit,
      offset,
    });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch pending deposits" });
  }
}

async function getPendingWithdrawals(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const transactions = await walletUsecase.getPendingWithdrawals(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      transactions,
      count: transactions.length,
      limit,
      offset,
    });
  } catch (err) {
    return res
      .status(500)
      .json({ error: "Failed to fetch pending withdrawals" });
  }
}

async function getCompletedDeposits(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const transactions = await walletUsecase.getCompletedDeposits(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      transactions,
      count: transactions.length,
      limit,
      offset,
    });
  } catch (err) {
    return res
      .status(500)
      .json({ error: "Failed to fetch completed deposits" });
  }
}

async function getCompletedWithdrawals(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const transactions = await walletUsecase.getCompletedWithdrawals(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      transactions,
      count: transactions.length,
      limit,
      offset,
    });
  } catch (err) {
    return res
      .status(500)
      .json({ error: "Failed to fetch completed withdrawals" });
  }
}

async function getFailedTransactions(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const transactions = await walletUsecase.getFailedTransactions(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      transactions,
      count: transactions.length,
      limit,
      offset,
    });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch failed transactions" });
  }
}

async function getTransferTransactions(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const transactions = await walletUsecase.getTransferTransactions(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      transactions,
      count: transactions.length,
      limit,
      offset,
    });
  } catch (err) {
    return res
      .status(500)
      .json({ error: "Failed to fetch transfer transactions" });
  }
}

async function getAllTransactions(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const transactions = await walletUsecase.getAllTransactions(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      transactions,
      count: transactions.length,
      limit,
      offset,
    });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch transactions" });
  }
}

async function getDashboardStats(req, res) {
  try {
    const stats = await walletUsecase.getDashboardStats(req.app.locals.db);
    return res.status(200).json(stats);
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch dashboard stats" });
  }
}

module.exports = {
  deposit,
  withdraw,
  transfer,
  getWallet,
  getWalletByTelegramId,
  getDepositHistory,
  getWithdrawalHistory,
  getTransferHistory,
  approveDeposit,
  rejectDeposit,
  approveWithdrawal,
  rejectWithdrawal,
  cancelTransaction,
  getPendingDeposits,
  getPendingWithdrawals,
  getCompletedDeposits,
  getCompletedWithdrawals,
  getFailedTransactions,
  getTransferTransactions,
  getAllTransactions,
  getDashboardStats,
};
