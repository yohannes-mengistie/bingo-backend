const userRepository = require("../repositories/userRepository");
const walletRepository = require("../repositories/walletRepository");
const transactionRepository = require("../repositories/transactionRepository");
const gameRepository = require("../repositories/gameRepository");
const transactionService = require("../services/transactionService");
const {
  TransactionType,
  TransactionStatus,
  PaymentMethod,
  DEFAULT_HISTORY_LIMIT,
} = require("../constants/transactions");

async function deposit(pool, request) {
  if (request.amount <= 0) {
    throw new Error("amount must be greater than 0");
  }

  const user = await userRepository.findById(pool, request.user_id);
  if (!user) {
    throw new Error("user not found");
  }

  const transaction = await transactionRepository.createTransaction(pool, {
    user_id: request.user_id,
    type: TransactionType.Deposit,
    amount: request.amount,
    status: TransactionStatus.Pending,
    transaction_type: request.transaction_type,
    transaction_id: request.transaction_id,
  });

  return transaction;
}

async function withdraw(pool, request) {
  if (request.amount <= 0) {
    throw new Error("amount must be greater than 0");
  }

  if (
    request.account_type !== PaymentMethod.CBE &&
    request.account_type !== PaymentMethod.Telebirr
  ) {
    throw new Error("account_type must be either CBE or Telebirr");
  }

  if (!request.account_number) {
    throw new Error("account_number is required");
  }

  const user = await userRepository.findById(pool, request.user_id);
  if (!user) {
    throw new Error("user not found");
  }

  return transactionService.processWithdrawal(
    pool,
    request.user_id,
    request.amount,
    request.account_number,
    request.account_type
  );
}

async function transfer(pool, request) {
  if (request.amount <= 0) {
    throw new Error("amount must be greater than 0");
  }

  if (request.sender_id === request.receiver_id) {
    throw new Error("cannot transfer to yourself");
  }

  const sender = await userRepository.findById(pool, request.sender_id);
  if (!sender) {
    throw new Error("sender not found");
  }

  const receiver = await userRepository.findById(pool, request.receiver_id);
  if (!receiver) {
    throw new Error("receiver not found");
  }

  const result = await transactionService.processTransfer(
    pool,
    request.sender_id,
    request.receiver_id,
    request.amount
  );

  return {
    senderTransaction: result.senderTransaction,
    receiverTransaction: result.receiverTransaction,
  };
}

async function getWallet(pool, userId) {
  const wallet = await walletRepository.findByUserId(pool, userId);
  if (!wallet) {
    throw new Error("wallet not found");
  }

  return wallet;
}

async function getWalletByTelegramId(pool, telegramId) {
  const user = await userRepository.findByTelegramId(pool, telegramId);
  if (!user) {
    throw new Error("user not found");
  }

  const wallet = await walletRepository.findByUserId(pool, user.id);
  if (!wallet) {
    throw new Error("wallet not found");
  }

  return wallet;
}

async function approveDeposit(pool, transactionId) {
  return transactionService.approveDeposit(pool, transactionId);
}

async function rejectDeposit(pool, transactionId) {
  return transactionService.rejectDeposit(pool, transactionId);
}

async function approveWithdrawal(pool, transactionId) {
  return transactionService.approveWithdrawal(pool, transactionId);
}

async function rejectWithdrawal(pool, transactionId) {
  return transactionService.rejectWithdrawal(pool, transactionId);
}

async function cancelTransaction(pool, transactionId) {
  return transactionService.cancelTransaction(pool, transactionId);
}

async function getDepositHistory(pool, userId, limit) {
  const finalLimit = limit > 0 ? limit : DEFAULT_HISTORY_LIMIT;
  return transactionRepository.findByUserIdAndType(
    pool,
    userId,
    TransactionType.Deposit,
    finalLimit
  );
}

async function getWithdrawalHistory(pool, userId, limit) {
  const finalLimit = limit > 0 ? limit : DEFAULT_HISTORY_LIMIT;
  return transactionRepository.findByUserIdAndType(
    pool,
    userId,
    TransactionType.Withdraw,
    finalLimit
  );
}

async function getTransferHistory(pool, userId, limit) {
  const finalLimit = limit > 0 ? limit : DEFAULT_HISTORY_LIMIT;
  const transactions = await transactionRepository.findByUserIdAndTypes(
    pool,
    userId,
    [TransactionType.TransferIn, TransactionType.TransferOut],
    finalLimit
  );

  const userIds = new Set();
  for (const tx of transactions) {
    if (tx.reference) {
      userIds.add(tx.reference);
    }
  }

  const usersMap = new Map();
  for (const otherUserId of userIds) {
    const user = await userRepository.findById(pool, otherUserId);
    if (user) {
      usersMap.set(otherUserId, user);
    }
  }

  return transactions.map((tx) => {
    const entry = { transaction: tx };
    if (tx.reference && usersMap.has(tx.reference)) {
      entry.to = usersMap.get(tx.reference);
    }
    return entry;
  });
}

async function getPendingDeposits(pool, limit, offset) {
  return transactionRepository.findByStatusAndType(
    pool,
    TransactionStatus.Pending,
    TransactionType.Deposit,
    limit,
    offset
  );
}

async function getPendingWithdrawals(pool, limit, offset) {
  return transactionRepository.findByStatusAndType(
    pool,
    TransactionStatus.Pending,
    TransactionType.Withdraw,
    limit,
    offset
  );
}

async function getCompletedDeposits(pool, limit, offset) {
  return transactionRepository.findByStatusAndType(
    pool,
    TransactionStatus.Completed,
    TransactionType.Deposit,
    limit,
    offset
  );
}

async function getCompletedWithdrawals(pool, limit, offset) {
  return transactionRepository.findByStatusAndType(
    pool,
    TransactionStatus.Completed,
    TransactionType.Withdraw,
    limit,
    offset
  );
}

async function getFailedTransactions(pool, limit, offset) {
  return transactionRepository.findByStatus(
    pool,
    TransactionStatus.Failed,
    limit,
    offset
  );
}

async function getTransferTransactions(pool, limit, offset) {
  return transactionRepository.findByTypes(
    pool,
    [TransactionType.TransferIn, TransactionType.TransferOut],
    limit,
    offset
  );
}

async function getAllTransactions(pool, limit, offset) {
  return transactionRepository.findAll(pool, limit, offset);
}

async function getDashboardStats(pool) {
  const pendingDeposits = await transactionRepository.countByStatusAndType(
    pool,
    TransactionStatus.Pending,
    TransactionType.Deposit
  );

  const pendingWithdrawals = await transactionRepository.countByStatusAndType(
    pool,
    TransactionStatus.Pending,
    TransactionType.Withdraw
  );

  const totalUsers = await userRepository.countAll(pool);
  const totalTransactions = await transactionRepository.countAll(pool);
  const totalBalance = await walletRepository.getTotalBalance(pool);
  const gamesByType = await gameRepository.countGamesByType(pool);
  const totalHouseCut = await gameRepository.getTotalHouseCut(pool);

  return {
    pending_deposits: pendingDeposits,
    pending_withdrawals: pendingWithdrawals,
    total_users: totalUsers,
    total_transactions: totalTransactions,
    total_balance: totalBalance,
    games_by_type: gamesByType,
    total_house_cut: totalHouseCut,
  };
}

module.exports = {
  deposit,
  withdraw,
  transfer,
  getWallet,
  getWalletByTelegramId,
  approveDeposit,
  rejectDeposit,
  approveWithdrawal,
  rejectWithdrawal,
  cancelTransaction,
  getDepositHistory,
  getWithdrawalHistory,
  getTransferHistory,
  getPendingDeposits,
  getPendingWithdrawals,
  getCompletedDeposits,
  getCompletedWithdrawals,
  getFailedTransactions,
  getTransferTransactions,
  getAllTransactions,
  getDashboardStats,
};
