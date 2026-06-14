const { checkPasswordHash, hashPassword } = require("../auth/password");
const { generateToken } = require("../auth/jwtService");
const { generateReferralCode } = require("../utils/referral");
const userRepository = require("../repositories/userRepository");

const MAX_REFERRAL_ATTEMPTS = 10;

async function login(pool, request) {
  const user = await userRepository.findByTelegramId(
    pool,
    request.telegram_id
  );
  if (!user) {
    const error = new Error("invalid credentials");
    error.code = "UNAUTHORIZED";
    throw error;
  }

  if (user.role !== "admin") {
    const error = new Error("admin access required");
    error.code = "FORBIDDEN";
    throw error;
  }

  if (!user.password) {
    const error = new Error("password not set for this user");
    error.code = "UNAUTHORIZED";
    throw error;
  }

  const matches = await checkPasswordHash(request.password, user.password);
  if (!matches) {
    const error = new Error("invalid credentials");
    error.code = "UNAUTHORIZED";
    throw error;
  }

  const token = generateToken(user.id, user.role);
  delete user.password;

  return {
    token,
    user,
  };
}

async function createAdmin(pool, request, adminSecretCode) {
  if (!adminSecretCode) {
    throw new Error("secret code not configured");
  }
  if (request.secret_code !== adminSecretCode) {
    const error = new Error("invalid secret code");
    error.code = "FORBIDDEN";
    throw error;
  }

  const hashedPassword = await hashPassword(request.password);
  const existingUser = await userRepository.findByTelegramId(
    pool,
    request.telegram_id
  );

  if (!existingUser) {
    const referralCode = await generateUniqueReferralCode(pool);
    const newUser = await userRepository.createUser(pool, {
      telegramId: request.telegram_id,
      firstName: "Admin",
      lastName: null,
      phoneNumber: `tg_${request.telegram_id}`,
      referralCode,
      role: "admin",
      password: hashedPassword,
    });

    delete newUser.password;
    return newUser;
  }

  await userRepository.setAdminCredentialsByTelegramId(
    pool,
    request.telegram_id,
    hashedPassword
  );

  const updatedUser = await userRepository.findByTelegramId(
    pool,
    request.telegram_id
  );
  if (!updatedUser) {
    throw new Error("user not found");
  }

  delete updatedUser.password;
  return updatedUser;
}

async function generateUniqueReferralCode(pool) {
  for (let i = 0; i < MAX_REFERRAL_ATTEMPTS; i += 1) {
    const code = generateReferralCode();
    const existing = await userRepository.findByReferralCode(pool, code);
    if (!existing) {
      return code;
    }

    if (i === MAX_REFERRAL_ATTEMPTS - 1) {
      throw new Error(
        "failed to generate unique referral code after multiple attempts"
      );
    }
  }

  throw new Error("failed to generate referral code");
}

module.exports = {
  login,
  createAdmin,
};
