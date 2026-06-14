const { validateToken } = require("../auth/jwtService");

const USER_ID_KEY = "user_id";
const ROLE_KEY = "role";

function authMiddleware(req, res, next) {
  const authHeader = req.headers.authorization || "";
  if (!authHeader) {
    return res.status(401).json({
      error: "Authorization header required",
    });
  }

  const parts = authHeader.split(" ");
  if (parts.length !== 2 || parts[0] !== "Bearer") {
    return res.status(401).json({
      error: "Invalid authorization header format. Expected: Bearer <token>",
    });
  }

  try {
    const claims = validateToken(parts[1]);
    req.auth = {
      [USER_ID_KEY]: claims.user_id,
      [ROLE_KEY]: claims.role,
    };
    return next();
  } catch (err) {
    return res.status(401).json({
      error: "Invalid or expired token",
    });
  }
}

function adminMiddleware(req, res, next) {
  const role = req.auth ? req.auth[ROLE_KEY] : null;
  if (!role) {
    return res.status(401).json({
      error: "User role not found",
    });
  }

  if (role !== "admin") {
    return res.status(403).json({
      error: "Admin access required",
    });
  }

  return next();
}

module.exports = {
  authMiddleware,
  adminMiddleware,
  USER_ID_KEY,
  ROLE_KEY,
};
