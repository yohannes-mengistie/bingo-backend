const { generateReferralCode } = require("../utils/referral");
const userRepository = require("../repositories/userRepository");
const walletRepository = require("../repositories/walletRepository");
const { normalizePhoneNumber } = require("../utils/phone");

const DEFAULT_USER_BALANCE = 5.0;
const MAX_REFERRAL_ATTEMPTS = 10;

async function createUser(pool, request) {
  const normalizedPhone = normalizePhoneNumber(request.phone);

  const existingByTelegram = await userRepository.findByTelegramId(
    pool,
    request.telegram_id
  );
  if (existingByTelegram) {
    const error = new Error("user with this telegram ID already exists");
    error.code = "CONFLICT";
    throw error;
  }

  const existingByPhone = await userRepository.findByPhone(pool, normalizedPhone);
  if (existingByPhone) {
    const error = new Error("user with this phone number already exists");
    error.code = "CONFLICT";
    throw error;
  }

  let referralCode = "";
  for (let i = 0; i < MAX_REFERRAL_ATTEMPTS; i += 1) {
    const code = generateReferralCode();
    const existing = await userRepository.findByReferralCode(pool, code);
    if (!existing) {
      referralCode = code;
      break;
    }

    if (i === MAX_REFERRAL_ATTEMPTS - 1) {
      throw new Error(
        "failed to generate unique referral code after multiple attempts"
      );
    }
  }

  const client = await pool.connect();
  try {
    await client.query("BEGIN");

    const user = await userRepository.createUser(client, {
      telegramId: request.telegram_id,
      firstName: request.first_name,
      lastName: request.last_name || null,
      phoneNumber: normalizedPhone,
      referralCode,
    });

    const wallet = await walletRepository.createWallet(client, {
      userId: user.id,
      balance: DEFAULT_USER_BALANCE,
      demoBalance: 0.0,
    });

    await client.query("COMMIT");

    return { user, wallet };
  } catch (err) {
    await client.query("ROLLBACK");
    throw err;
  } finally {
    client.release();
  }
}

module.exports = {
  createUser,
  findUserByTelegramId,
  findUserByPhone,
  findUserByReferralCode,
  updateUserName,
  getAllUsersWithWallets,
};

async function findUserByTelegramId(pool, telegramId) {
  const user = await userRepository.findByTelegramId(pool, telegramId);
  if (!user) {
    throw new Error("user not found");
  }
  delete user.password;
  return user;
}

async function findUserByPhone(pool, phone) {
  const normalizedPhone = normalizePhoneNumber(phone);
  const user = await userRepository.findByPhone(pool, normalizedPhone);
  if (!user) {
    throw new Error("user not found");
  }
  delete user.password;
  return user;
}

async function findUserByReferralCode(pool, referralCode) {
  const user = await userRepository.findByReferralCode(pool, referralCode);
  if (!user) {
    throw new Error("user not found");
  }
  delete user.password;
  return user;
}

async function updateUserName(pool, userId, request) {
  const user = await userRepository.findById(pool, userId);
  if (!user) {
    throw new Error("user not found");
  }

  const firstName = request.first_name;
  const lastName = request.last_name ?? null;

  const updated = await userRepository.update(pool, userId, {
    first_name: firstName,
    last_name: lastName,
    phone_number: user.phone_number,
  });

  delete updated.password;
  return updated;
}

async function getAllUsersWithWallets(pool, limit, offset) {
  const users = await userRepository.findAll(pool, limit, offset);
  const totalCount = await userRepository.countAll(pool);

  const usersWithWallets = [];
  for (const user of users) {
    const wallet = await walletRepository.findByUserId(pool, user.id);
    usersWithWallets.push({
      ...user,
      password: undefined,
      wallet: wallet || null,
    });
  }

  return { users: usersWithWallets, totalCount };
}
