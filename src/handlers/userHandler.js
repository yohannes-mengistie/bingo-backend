const userUsecase = require("../usecases/userUsecase");
const { normalizePhoneNumber } = require("../utils/phone");
const { getPaginationParams } = require("../utils/pagination");

const uuidRegex =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function isUuid(value) {
  return typeof value === "string" && uuidRegex.test(value);
}

function isValidRegisterBody(body) {
  return (
    body &&
    typeof body.telegram_id === "number" &&
    typeof body.first_name === "string" &&
    body.first_name.trim().length > 0 &&
    typeof body.phone === "string" &&
    body.phone.trim().length > 0
  );
}

async function registerUser(req, res) {
  if (!isValidRegisterBody(req.body)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const { user, wallet } = await userUsecase.createUser(req.app.locals.db, req.body);

    return res.status(201).json({
      message: "User and wallet created successfully",
      user,
      wallet,
    });
  } catch (err) {
    if (err.code === "CONFLICT") {
      return res.status(409).json({
        error: err.message,
      });
    }

    return res.status(500).json({
      error: err.message || "Internal server error",
    });
  }
}

async function findByTelegramId(req, res) {
  const telegramId = Number.parseInt(req.params.telegram_id, 10);
  if (!Number.isFinite(telegramId)) {
    return res.status(400).json({ error: "Invalid telegram ID" });
  }

  try {
    const user = await userUsecase.findUserByTelegramId(
      req.app.locals.db,
      telegramId
    );
    return res.status(200).json({ user });
  } catch (err) {
    return res.status(404).json({ error: "User not found" });
  }
}

async function findByPhone(req, res) {
  const phone = typeof req.query.phone === "string" ? req.query.phone : "";
  if (!phone) {
    return res.status(400).json({ error: "phone parameter is required" });
  }

  try {
    const user = await userUsecase.findUserByPhone(
      req.app.locals.db,
      normalizePhoneNumber(phone)
    );
    return res.status(200).json({ user });
  } catch (err) {
    return res.status(404).json({ error: "User not found" });
  }
}

async function findByReferralCode(req, res) {
  const referralCode = req.params.referral_code;
  if (!referralCode) {
    return res.status(400).json({ error: "referral_code is required" });
  }

  try {
    const user = await userUsecase.findUserByReferralCode(
      req.app.locals.db,
      referralCode
    );
    return res.status(200).json({ user });
  } catch (err) {
    return res.status(404).json({ error: "User not found" });
  }
}

async function updateUserName(req, res) {
  const userId = req.params.user_id;
  if (!isUuid(userId)) {
    return res.status(400).json({ error: "Invalid user ID" });
  }

  if (!req.body || typeof req.body.first_name !== "string") {
    return res.status(400).json({ error: "Invalid request data" });
  }

  try {
    const user = await userUsecase.updateUserName(
      req.app.locals.db,
      userId,
      req.body
    );
    return res.status(200).json({
      message: "User name updated successfully",
      user,
    });
  } catch (err) {
    if (err.message === "user not found") {
      return res.status(404).json({ error: err.message });
    }
    return res.status(500).json({ error: err.message });
  }
}

async function getAllUsers(req, res) {
  const { limit, offset } = getPaginationParams(req.query);
  try {
    const result = await userUsecase.getAllUsersWithWallets(
      req.app.locals.db,
      limit,
      offset
    );
    return res.status(200).json({
      users: result.users,
      count: result.totalCount,
      limit,
      offset,
    });
  } catch (err) {
    return res.status(500).json({ error: "Failed to fetch users" });
  }
}

module.exports = {
  registerUser,
  findByTelegramId,
  findByPhone,
  findByReferralCode,
  updateUserName,
  getAllUsers,
};
