const crypto = require("crypto");

function generateReferralCode() {
  const bytes = crypto.randomBytes(6);
  const code = bytes
    .toString("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "")
    .slice(0, 8)
    .toUpperCase();

  return code;
}

module.exports = {
  generateReferralCode,
};
