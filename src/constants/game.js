const GameType = {
  G1: "G1",
  G2: "G2",
  G3: "G3",
  G4: "G4",
  G5: "G5",
  G6: "G6",
  G7: "G7",
};

const GameState = {
  Waiting: "WAITING",
  Countdown: "COUNTDOWN",
  Drawing: "DRAWING",
  Finished: "FINISHED",
  Closed: "CLOSED",
  Cancelled: "CANCELLED",
};

const BingoLetter = {
  B: "B",
  I: "I",
  N: "N",
  G: "G",
  O: "O",
};

const MinPlayers = 2;
const HouseCut = 0.2;

const CountdownDurationSeconds = 60;
const CountdownTickerIntervalMs = 1000;

const MinCardId = 1;
const MaxCardId = 200;
const TotalCards = 200;
const CardGridSize = 5;
const CardTotalPositions = CardGridSize * CardGridSize;
const CardCenterValue = 0;
const CardCenterRow = 2;
const CardCenterCol = 2;

const BingoNumberMinB = 1;
const BingoNumberMaxB = 15;
const BingoNumberMinI = 16;
const BingoNumberMaxI = 30;
const BingoNumberMinN = 31;
const BingoNumberMaxN = 45;
const BingoNumberMinG = 46;
const BingoNumberMaxG = 60;
const BingoNumberMinO = 61;
const BingoNumberMaxO = 75;

const NumbersPerLetter = 15;

const BetAmount = {
  G1: 5.0,
  G2: 7.0,
  G3: 10.0,
  G4: 20.0,
  G5: 50.0,
  G6: 100.0,
  G7: 200.0,
};

const WebSocketEvent = {
  PlayerJoined: "PLAYER_JOINED",
  PlayerLeft: "PLAYER_LEFT",
  GameStatus: "GAME_STATUS",
  Countdown: "COUNTDOWN",
  NumberDrawn: "NUMBER_DRAWN",
  Winner: "WINNER",
  PlayerEliminated: "PLAYER_ELIMINATED",
  NewGameAvailable: "NEW_GAME_AVAILABLE",
  InitialState: "INITIAL_STATE",
};

const MaxAvailableGamesLimit = 50;

const DrawIntervalMs = 3000;

const WebSocketInitialStateTimeoutMs = 5000;

function getBetAmount(gameType) {
  return BetAmount[gameType] || 0;
}

module.exports = {
  GameType,
  GameState,
  BingoLetter,
  MinPlayers,
  HouseCut,
  CountdownDurationSeconds,
  CountdownTickerIntervalMs,
  MinCardId,
  MaxCardId,
  TotalCards,
  CardGridSize,
  CardTotalPositions,
  CardCenterValue,
  CardCenterRow,
  CardCenterCol,
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
  NumbersPerLetter,
  BetAmount,
  WebSocketEvent,
  MaxAvailableGamesLimit,
  DrawIntervalMs,
  WebSocketInitialStateTimeoutMs,
  getBetAmount,
};
