// Shared state (globals)
let lastNetwork = null;
let lastTime = null;
let lastData = null;

let currentSort = { column: 'memory_percent', direction: 'desc' };

// Processes page state
let currentProcessSort = 'memory'; // 'memory', 'cpu', 'threads', 'uptime', 'tree'
let currentProcessSearch = '';
let allProcesses = [];

// Sensors ordering (persisted)
const SENSOR_ORDER_KEY = 'sensorOrder';
let lastSensorItemKeys = [];
