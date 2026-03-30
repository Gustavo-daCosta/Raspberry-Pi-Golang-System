#include <WiFi.h>
#include <HTTPClient.h>
#include <WiFiUdp.h>
#include <NTPClient.h>
#include <time.h>
#include <stdio.h>

// Network configuration
const char* WIFI_SSID = "SHARE-RESIDENTE";
const char* WIFI_PASSWORD = "Share@residente23";
const char* API_URL = "http://10.254.18.187:8080/telemetry";

// Device and telemetry configuration
const int DEVICE_ID = 1;
const unsigned long WIFI_RECONNECT_INTERVAL_MS = 5000;
const unsigned long NTP_UPDATE_INTERVAL_MS = 60000;

// GPIO configuration
const int BUTTON_PIN = 15;
const int LDR_PIN = 26;

// ADC smoothing configuration
const int ADC_RESOLUTION_BITS = 12;
const int ADC_MAX_VALUE = 4095;
const float ADC_REF_VOLTAGE = 3.3f;
const int MOVING_WINDOW = 10;

// Debounce configuration
const unsigned long DEBOUNCE_US = 50000; // 50ms

volatile bool buttonInterruptFlag = false;
volatile unsigned long lastButtonInterruptUs = 0;
int lastButtonStableState = LOW;

int readings[MOVING_WINDOW];
int readIndex = 0;
long runningTotal = 0;
int smoothedRaw = 0;

unsigned long lastWifiReconnectMs = 0;

WiFiUDP ntpUDP;
NTPClient timeClient(ntpUDP, "pool.ntp.org", 0, NTP_UPDATE_INTERVAL_MS);

void handleButtonInterrupt();
void connectToWifi(bool blocking);
void ensureWifiConnected();
void syncClockIfNeeded(bool force);
void updateAnalogMovingAverage();
float adcRawToVoltage(int rawValue);
void buildRFC3339Timestamp(char* out, size_t outSize);
void buildTelemetryPayload(char* out, size_t outSize, const char* sensorType, const char* readingType, float value);
bool postSensorData(const char* payload);
void registerButtonEventIfNeeded();

void setup() {
  Serial.begin(115200);

  pinMode(BUTTON_PIN, INPUT_PULLUP);
  pinMode(LDR_PIN, INPUT);
  analogReadResolution(ADC_RESOLUTION_BITS);
  lastButtonStableState = digitalRead(BUTTON_PIN);

  for (int i = 0; i < MOVING_WINDOW; i++) {
    readings[i] = 0;
  }

  attachInterrupt(digitalPinToInterrupt(BUTTON_PIN), handleButtonInterrupt, CHANGE);
  connectToWifi(true);
  timeClient.begin();
  syncClockIfNeeded(true);
}

void loop() {
  ensureWifiConnected();
  syncClockIfNeeded(false);

  updateAnalogMovingAverage();
  char analogPayload[220];
  buildTelemetryPayload(analogPayload, sizeof(analogPayload), "ldr_sensor", "analogica", adcRawToVoltage(smoothedRaw));
  if (!postSensorData(analogPayload)) {
    Serial.println("[HTTP] falha no envio da telemetria analogica");
  }

  registerButtonEventIfNeeded();
  delay(20);
}

void handleButtonInterrupt() {
  unsigned long nowUs = micros();
  if (nowUs - lastButtonInterruptUs >= DEBOUNCE_US) {
    lastButtonInterruptUs = nowUs;
    buttonInterruptFlag = true;
  }
}

void connectToWifi(bool blocking) {
  Serial.print("[WIFI] conectando em ");
  Serial.println(WIFI_SSID);

  WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

  if (!blocking) {
    return;
  }

  unsigned long startedAt = millis();
  while (WiFi.status() != WL_CONNECTED && millis() - startedAt < 20000) {
    delay(400);
    Serial.print(".");
  }

  if (WiFi.status() == WL_CONNECTED) {
    Serial.println("\n[WIFI] conectado");
    Serial.print("[WIFI] IP: ");
    Serial.println(WiFi.localIP());
  } else {
    Serial.println("\n[WIFI] falha inicial, tentara novamente no loop");
  }
}

void ensureWifiConnected() {
  if (WiFi.status() == WL_CONNECTED) {
    return;
  }

  unsigned long now = millis();
  if (now - lastWifiReconnectMs < WIFI_RECONNECT_INTERVAL_MS) {
    return;
  }

  lastWifiReconnectMs = now;
  Serial.println("[WIFI] desconectado, tentando reconectar...");
  WiFi.disconnect();
  connectToWifi(false);
}

void updateAnalogMovingAverage() {
  runningTotal -= readings[readIndex];

  int raw = analogRead(LDR_PIN);
  readings[readIndex] = raw;
  runningTotal += readings[readIndex];

  readIndex++;
  if (readIndex >= MOVING_WINDOW) {
    readIndex = 0;
  }

  smoothedRaw = runningTotal / MOVING_WINDOW;

  Serial.print("[ADC] raw=");
  Serial.print(raw);
  Serial.print(" smoothed=");
  Serial.print(smoothedRaw);
  Serial.print(" voltage=");
  Serial.println(adcRawToVoltage(smoothedRaw), 3);
}

float adcRawToVoltage(int rawValue) {
  return (rawValue * ADC_REF_VOLTAGE) / ADC_MAX_VALUE;
}

bool postSensorData(const char* payload) {
  if (WiFi.status() != WL_CONNECTED) {
    return false;
  }

  HTTPClient http;
  http.begin(API_URL);
  http.addHeader("Content-Type", "application/json");
  int responseCode = http.POST(payload);
  http.end();

  if (responseCode >= 200 && responseCode < 300) {
    Serial.print("[HTTP] sucesso codigo=");
    Serial.println(responseCode);
    return true;
  }

  Serial.print("[HTTP] falha codigo=");
  Serial.println(responseCode);
  return false;
}

void syncClockIfNeeded(bool force) {
  if (WiFi.status() != WL_CONNECTED) {
    return;
  }

  static unsigned long lastNtpSyncMs = 0;
  bool shouldUpdate = force || (millis() - lastNtpSyncMs >= NTP_UPDATE_INTERVAL_MS);
  if (!shouldUpdate) {
    return;
  }

  if (timeClient.update() || timeClient.forceUpdate()) {
    lastNtpSyncMs = millis();
    Serial.println("[NTP] relogio sincronizado");
  } else {
    Serial.println("[NTP] falha ao sincronizar relogio");
  }
}

void buildRFC3339Timestamp(char* out, size_t outSize) {
  unsigned long epoch = timeClient.getEpochTime();
  if (epoch == 0) {
    snprintf(out, outSize, "1970-01-01T00:00:00Z");
    return;
  }

  time_t now = (time_t)epoch;
  struct tm utcTime;
  gmtime_r(&now, &utcTime);
  strftime(out, outSize, "%Y-%m-%dT%H:%M:%SZ", &utcTime);
}

void buildTelemetryPayload(char* out, size_t outSize, const char* sensorType, const char* readingType, float value) {
  char timestamp[32];
  buildRFC3339Timestamp(timestamp, sizeof(timestamp));

  snprintf(
    out,
    outSize,
    "{\"device_id\":%d,\"timestamp\":\"%s\",\"sensor_type\":\"%s\",\"reading_type\":\"%s\",\"value\":%.4f}",
    DEVICE_ID,
    timestamp,
    sensorType,
    readingType,
    value
  );
}

void registerButtonEventIfNeeded() {
  bool shouldProcess = false;
  noInterrupts();
  if (buttonInterruptFlag) {
    buttonInterruptFlag = false;
    shouldProcess = true;
  }
  interrupts();

  if (!shouldProcess) {
    return;
  }

  int currentState = digitalRead(BUTTON_PIN);
  if (currentState != lastButtonStableState) {
    lastButtonStableState = currentState;

    float value = (currentState == LOW) ? 1.0f : 0.0f;
    char payload[220];
    buildTelemetryPayload(payload, sizeof(payload), "push_button", "discreta", value);

    if (!postSensorData(payload)) {
      Serial.println("[HTTP] falha no envio da telemetria digital");
    }
  }
}
