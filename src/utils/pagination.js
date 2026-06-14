function getPaginationParams(query) {
  let limit = 50;
  let offset = 0;

  if (query && query.limit) {
    const parsedLimit = Number.parseInt(query.limit, 10);
    if (Number.isFinite(parsedLimit) && parsedLimit > 0) {
      limit = parsedLimit;
    }
  }

  if (query && query.offset) {
    const parsedOffset = Number.parseInt(query.offset, 10);
    if (Number.isFinite(parsedOffset) && parsedOffset >= 0) {
      offset = parsedOffset;
    }
  }

  return { limit, offset };
}

module.exports = {
  getPaginationParams,
};
