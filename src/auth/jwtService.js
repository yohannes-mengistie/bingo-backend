const jwt = require("jsonwebtoken");
const config = require("../config");

function generateToken(userId, role) {
  const payload = {
    user_id: userId,
    role,
  };

  return jwt.sign(payload, config.jwt.secret, {
    algorithm: "HS256",
    expiresIn: `${config.jwt.expirationHours}h`,
  });
}

function validateToken(token) {
  return jwt.verify(token, config.jwt.secret, {
    algorithms: ["HS256"],
  });
}

module.exports = {
  generateToken,
  validateToken,
};
