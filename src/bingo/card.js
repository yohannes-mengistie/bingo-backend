const cardData = require("./cardData");
const {
  MinCardId,
  MaxCardId,
  TotalCards,
  CardGridSize,
  CardCenterCol,
  CardCenterRow,
  CardCenterValue,
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

function generateCard(cardId) {
  if (cardId < MinCardId || cardId > MaxCardId) {
    return null;
  }

  const index = cardId - MinCardId;
  const data = cardData[index];
  if (!data) {
    return null;
  }

  const numbers = data.map((row) => row.slice());
  return {
    id: cardId,
    numbers,
  };
}

function generateAllCards() {
  const cards = new Array(TotalCards);
  for (let i = MinCardId; i <= MaxCardId; i += 1) {
    cards[i - MinCardId] = generateCard(i);
  }
  return cards;
}

function validateBingo(card, markedNumbers) {
  if (!markedNumbers || markedNumbers.length < 4) {
    return false;
  }

  const markedSet = new Set(markedNumbers);

  for (let row = 0; row < CardGridSize; row += 1) {
    let count = 0;
    for (let col = 0; col < CardGridSize; col += 1) {
      const num = card.numbers[row][col];
      if (num === CardCenterValue || markedSet.has(num)) {
        count += 1;
      }
    }
    if (count === CardGridSize) {
      return true;
    }
  }

  for (let col = 0; col < CardGridSize; col += 1) {
    let count = 0;
    for (let row = 0; row < CardGridSize; row += 1) {
      const num = card.numbers[row][col];
      if (num === CardCenterValue || markedSet.has(num)) {
        count += 1;
      }
    }
    if (count === CardGridSize) {
      return true;
    }
  }

  let count = 0;
  for (let i = 0; i < CardGridSize; i += 1) {
    const num = card.numbers[i][i];
    if (num === CardCenterValue || markedSet.has(num)) {
      count += 1;
    }
  }
  if (count === CardGridSize) {
    return true;
  }

  count = 0;
  for (let i = 0; i < CardGridSize; i += 1) {
    const num = card.numbers[i][CardGridSize - 1 - i];
    if (num === CardCenterValue || markedSet.has(num)) {
      count += 1;
    }
  }
  if (count === CardGridSize) {
    return true;
  }

  const corners = [
    [0, 0],
    [0, CardGridSize - 1],
    [CardGridSize - 1, 0],
    [CardGridSize - 1, CardGridSize - 1],
  ];

  count = 0;
  for (const [row, col] of corners) {
    const num = card.numbers[row][col];
    if (num === CardCenterValue || markedSet.has(num)) {
      count += 1;
    }
  }

  return count === corners.length;
}

function getLetterForNumber(num) {
  if (num >= BingoNumberMinB && num <= BingoNumberMaxB) {
    return "B";
  }
  if (num >= BingoNumberMinI && num <= BingoNumberMaxI) {
    return "I";
  }
  if (num >= BingoNumberMinN && num <= BingoNumberMaxN) {
    return "N";
  }
  if (num >= BingoNumberMinG && num <= BingoNumberMaxG) {
    return "G";
  }
  if (num >= BingoNumberMinO && num <= BingoNumberMaxO) {
    return "O";
  }
  return "";
}

module.exports = {
  generateCard,
  generateAllCards,
  validateBingo,
  getLetterForNumber,
};
