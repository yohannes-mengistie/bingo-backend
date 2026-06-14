const config = require("../config");
const authUsecase = require("../usecases/authUsecase");

function isValidLoginBody(body) {
  return (
    body &&
    typeof body.telegram_id === "number" &&
    typeof body.password === "string" &&
    body.password.length > 0
  );
}

function isValidCreateAdminBody(body) {
  return (
    body &&
    typeof body.telegram_id === "number" &&
    typeof body.password === "string" &&
    body.password.length >= 8 &&
    typeof body.secret_code === "string" &&
    body.secret_code.length > 0
  );
}

async function login(req, res) {
  if (!isValidLoginBody(req.body)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const response = await authUsecase.login(req.app.locals.db, req.body);
    return res.status(200).json(response);
  } catch (err) {
    if (err.code === "FORBIDDEN") {
      return res.status(403).json({
        error: err.message,
      });
    }

    return res.status(401).json({
      error: err.message,
    });
  }
}

async function createAdmin(req, res) {
  if (!isValidCreateAdminBody(req.body)) {
    return res.status(400).json({
      error: "Invalid request data",
    });
  }

  try {
    const user = await authUsecase.createAdmin(
      req.app.locals.db,
      req.body,
      config.adminSecretCode
    );

    return res.status(201).json({
      message: "Admin user created successfully",
      user,
    });
  } catch (err) {
    if (err.code === "FORBIDDEN" || err.message === "invalid secret code") {
      return res.status(403).json({
        error: err.message,
      });
    }

    if (err.message === "user not found") {
      return res.status(404).json({
        error: err.message,
      });
    }

    return res.status(500).json({
      error: err.message,
    });
  }
}

module.exports = {
  login,
  createAdmin,
};
