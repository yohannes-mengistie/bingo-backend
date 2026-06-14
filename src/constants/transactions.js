const TransactionType = {
  Deposit: "deposit",
  Withdraw: "withdraw",
  TransferIn: "transfer_in",
  TransferOut: "transfer_out",
};

const TransactionStatus = {
  Pending: "pending",
  Completed: "completed",
  Failed: "failed",
  Cancelled: "cancelled",
};

const PaymentMethod = {
  CBE: "CBE",
  Telebirr: "Telebirr",
};

const DEFAULT_HISTORY_LIMIT = 10;
const MIN_WITHDRAWAL_REMAINING_BALANCE = 10;

module.exports = {
  TransactionType,
  TransactionStatus,
  PaymentMethod,
  DEFAULT_HISTORY_LIMIT,
  MIN_WITHDRAWAL_REMAINING_BALANCE,
};
