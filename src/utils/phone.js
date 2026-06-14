function normalizePhoneNumber(phone) {
  if (!phone) {
    return "";
  }

  const normalized = String(phone).replace(/\D/g, "");
  return normalized.replace(/^0+/, "");
}

module.exports = {
  normalizePhoneNumber,
};
