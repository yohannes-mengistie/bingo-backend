const crypto = require("crypto");
const {
  BingoLetter,
  BingoNumberMinB,
  BingoNumberMaxB,
  BingoNumberMinI,
  BingoNumberMaxI,
  BingoNumberMinN,
  BingoNumberMaxN,
  BingoNumberMinG,
  BingoNumberMaxG,
  BingoNumberMinO,
  BingoNumberMaxO,
} = require("../constants/game");

function drawNumber(letter, drawnNumbers) {
  let min = 0;
  let max = 0;

  switch (letter) {
    case BingoLetter.B:
      min = BingoNumberMinB;
      max = BingoNumberMaxB;
      break;
    case BingoLetter.I:
      min = BingoNumberMinI;
      max = BingoNumberMaxI;
      break;
    case BingoLetter.N:
      min = BingoNumberMinN;
      max = BingoNumberMaxN;
      break;
    case BingoLetter.G:
      min = BingoNumberMinG;
      max = BingoNumberMaxG;
      break;
    case BingoLetter.O:
      min = BingoNumberMinO;
      max = BingoNumberMaxO;
      break;
    default:
      return 0;
  }

  const drawnSet = new Set(drawnNumbers || []);
  const available = [];
  for (let num = min; num <= max; num += 1) {
    if (!drawnSet.has(num)) {
      available.push(num);
    }
  }

  if (available.length === 0) {
    return 0;
  }

  const idx = crypto.randomInt(0, available.length);
  return available[idx];
}

function drawNextNumber(drawnNumbers) {
  const letters = [
    BingoLetter.B,
    BingoLetter.I,
    BingoLetter.N,
    BingoLetter.G,
    BingoLetter.O,
  ];

  const drawnSet = new Set(drawnNumbers || []);
  const availableLetters = [];

  for (const letter of letters) {
    let min = 0;
    let max = 0;
    switch (letter) {
      case BingoLetter.B:
        min = BingoNumberMinB;
        max = BingoNumberMaxB;
        break;
      case BingoLetter.I:
        min = BingoNumberMinI;
        max = BingoNumberMaxI;
        break;
      case BingoLetter.N:
        min = BingoNumberMinN;
        max = BingoNumberMaxN;
        break;
      case BingoLetter.G:
        min = BingoNumberMinG;
        max = BingoNumberMaxG;
        break;
      case BingoLetter.O:
        min = BingoNumberMinO;
        max = BingoNumberMaxO;
        break;
      default:
        break;
    }

    let hasAvailable = false;
    for (let num = min; num <= max; num += 1) {
      if (!drawnSet.has(num)) {
        hasAvailable = true;
        break;
      }
    }

    if (hasAvailable) {
      availableLetters.push(letter);
    }
  }

  if (availableLetters.length === 0) {
    return { letter: "", number: 0 };
  }

  const letter = availableLetters[crypto.randomInt(0, availableLetters.length)];
  const number = drawNumber(letter, drawnNumbers);

  return { letter, number };
}

module.exports = {
  drawNumber,
  drawNextNumber,
};
