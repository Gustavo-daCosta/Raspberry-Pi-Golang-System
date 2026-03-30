import http from 'k6/http';
import { check } from 'k6';

const scenario = {
  executor: 'constant-arrival-rate',
  rate: 250,
  timeUnit: '1s',
  duration: '1m',
  preAllocatedVUs: 500,
  maxVUs: 10000,
};

export const options = {
  scenarios: {
    telemetry_load: scenario,
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

const API_URL = __ENV.API_URL || 'http://localhost:8080';

function randomReadingType() {
  return Math.random() > 0.3 ? 'analogica' : 'discreta';
}

function randomSensorType(readingType) {
  if (readingType === 'discreta') {
    const sensors = ['presenca', 'porta', 'alarme'];
    return sensors[Math.floor(Math.random() * sensors.length)];
  }

  const sensors = ['temperatura', 'umidade', 'luminosidade', 'vibracao', 'nivel'];
  return sensors[Math.floor(Math.random() * sensors.length)];
}

function randomValue(readingType) {
  if (readingType === 'discreta') {
    return Math.random() > 0.5 ? 1 : 0;
  }

  return Number((Math.random() * 100).toFixed(2));
}

export default function () {
  const readingType = randomReadingType();

  const payload = {
    device_id: Math.floor(Math.random() * 1000) + 1,
    timestamp: new Date().toISOString(),
    sensor_type: randomSensorType(readingType),
    reading_type: readingType,
    value: randomValue(readingType),
  };

  const res = http.post(`${API_URL}/telemetry`, JSON.stringify(payload), {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, {
    'status is 202': (r) => r.status === 202,
  });
}
