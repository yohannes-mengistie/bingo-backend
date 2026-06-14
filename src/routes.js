const express = require("express");
const {
	registerUser,
	findByTelegramId,
	findByPhone,
	findByReferralCode,
	updateUserName,
	getAllUsers,
} = require("./handlers/userHandler");
const { login, createAdmin } = require("./handlers/authHandler");
const walletHandler = require("./handlers/walletHandler");
const gameHandler = require("./handlers/gameHandler");
const { authMiddleware, adminMiddleware } = require("./middleware/auth");

const router = express.Router();

router.post("/api/v1/user/register", registerUser);
router.get("/api/v1/user/telegram/:telegram_id", findByTelegramId);
router.get("/api/v1/user/phone", findByPhone);
router.get("/api/v1/user/referral/:referral_code", findByReferralCode);
router.put("/api/v1/user/:user_id/name", updateUserName);
router.post("/api/v1/auth/login", login);
router.post("/api/v1/auth/create-admin", createAdmin);

router.post("/api/v1/wallet/deposit", walletHandler.deposit);
router.post("/api/v1/wallet/withdraw", walletHandler.withdraw);
router.post("/api/v1/wallet/transfer", walletHandler.transfer);
router.get("/api/v1/wallet/telegram/:telegram_id", walletHandler.getWalletByTelegramId);
router.get("/api/v1/wallet/:user_id", walletHandler.getWallet);
router.get("/api/v1/wallet/:user_id/deposits", walletHandler.getDepositHistory);
router.get("/api/v1/wallet/:user_id/withdrawals", walletHandler.getWithdrawalHistory);
router.get("/api/v1/wallet/:user_id/transfers", walletHandler.getTransferHistory);

router.get("/api/v1/games", gameHandler.getGames);
router.get("/api/v1/games/user/:user_id/history", gameHandler.getGameHistory);
router.get("/api/v1/games/:gameId/state", gameHandler.getGameState);
router.get("/api/v1/games/:gameId/players/:userId", gameHandler.getPlayerInGame);
router.post("/api/v1/games/:gameId/join", gameHandler.joinGame);
router.post("/api/v1/games/:gameId/leave", gameHandler.leaveGame);
router.post("/api/v1/games/:gameId/bingo", gameHandler.claimBingo);

router.get("/api/v1/cards/:cardId", gameHandler.getCardData);

const adminRouter = express.Router();
adminRouter.use(authMiddleware, adminMiddleware);

adminRouter.get("/stats/dashboard", walletHandler.getDashboardStats);

adminRouter.get("/users", getAllUsers);

adminRouter.get("/transactions", walletHandler.getAllTransactions);
adminRouter.get("/transactions/pending/deposits", walletHandler.getPendingDeposits);
adminRouter.get("/transactions/pending/withdrawals", walletHandler.getPendingWithdrawals);
adminRouter.get("/transactions/completed/deposits", walletHandler.getCompletedDeposits);
adminRouter.get(
	"/transactions/completed/withdrawals",
	walletHandler.getCompletedWithdrawals
);
adminRouter.get("/transactions/failed", walletHandler.getFailedTransactions);
adminRouter.get("/transactions/transfers", walletHandler.getTransferTransactions);

adminRouter.post("/transactions/:id/approve-deposit", walletHandler.approveDeposit);
adminRouter.post("/transactions/:id/reject-deposit", walletHandler.rejectDeposit);
adminRouter.post(
	"/transactions/:id/approve-withdrawal",
	walletHandler.approveWithdrawal
);
adminRouter.post(
	"/transactions/:id/reject-withdrawal",
	walletHandler.rejectWithdrawal
);
adminRouter.post("/transactions/:id/cancel", walletHandler.cancelTransaction);

router.use("/api/v1/admin", adminRouter);

module.exports = router;
