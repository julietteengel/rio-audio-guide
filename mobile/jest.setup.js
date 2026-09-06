// Mock expo-sqlite for Jest testing with an in-memory database
jest.mock('expo-sqlite', () => {
  // Simple in-memory SQLite implementation for testing
  const databases = new Map();
  // Capture the last upsert SQL for test assertions
  const capturedState = { lastUpsertSql: null };
  // Expose for test inspection
  globalThis.__mockSQLiteCapture = capturedState;

  return {
    openDatabaseAsync: jest.fn(async (dbName) => {
      if (!databases.has(dbName)) {
        databases.set(dbName, {
          data: new Map(),
          userVersion: 0,
          tables: {},
        });
      }

      const db = databases.get(dbName);

      return {
        execAsync: jest.fn(async (sql) => {
          // Handle CREATE TABLE IF NOT EXISTS
          if (sql.includes('CREATE TABLE IF NOT EXISTS')) {
            // Parse table name and store it
            const match = sql.match(/CREATE TABLE IF NOT EXISTS (\w+)/);
            if (match) {
              const tableName = match[1];
              if (!db.tables[tableName]) {
                db.tables[tableName] = [];
              }
            }
          }
          // Handle ALTER TABLE
          if (sql.includes('ALTER TABLE')) {
            // No-op for mock
          }
          // Handle PRAGMA user_version
          if (sql.includes('PRAGMA user_version')) {
            // No-op for SET
          }
        }),

        getFirstAsync: jest.fn(async (sql, ...params) => {
          // Handle PRAGMA user_version
          if (sql.includes('PRAGMA user_version')) {
            return { user_version: db.userVersion };
          }

          // Handle SELECT queries
          if (sql.includes('SELECT') && sql.includes('FROM cached_places')) {
            const id = params[0];
            const row = db.data.get(id);
            if (row) {
              return row;
            }
            return undefined;
          }

          return undefined;
        }),

        getAllAsync: jest.fn(async (sql, ...params) => {
          // Handle SELECT * FROM cached_places
          if (sql.includes('SELECT * FROM cached_places')) {
            return Array.from(db.data.values());
          }
          return [];
        }),

        runAsync: jest.fn(async (sql, ...params) => {
          // Handle INSERT OR REPLACE with ON CONFLICT
          if (sql.includes('INSERT INTO cached_places')) {
            // Capture upsert SQL for test assertions
            if (sql.includes('ON CONFLICT')) {
              capturedState.lastUpsertSql = sql;
            }

            const id = params[0];
            const newRow = {
              id: params[0],
              name: params[1],
              category: params[2],
              lat: params[3],
              lon: params[4],
              body: params[5],
              audio_local_uri: params[6],
              last_notified_at: null,
            };

            if (db.data.has(id)) {
              // Upsert: update name/category/lat/lon/body (as per the real ON CONFLICT clause),
              // but preserve audio_local_uri and last_notified_at (as per the real clause exclusions)
              const existing = db.data.get(id);
              const merged = { ...existing };
              merged.name = newRow.name;
              merged.category = newRow.category;
              merged.lat = newRow.lat;
              merged.lon = newRow.lon;
              merged.body = newRow.body;
              // audio_local_uri and last_notified_at are NOT updated on conflict
              db.data.set(id, merged);
            } else {
              // Insert: new row
              db.data.set(id, newRow);
            }
          }

          // Handle UPDATE cached_places SET last_notified_at
          if (sql.includes('UPDATE cached_places SET last_notified_at')) {
            const timestamp = params[0];
            const id = params[1];
            const row = db.data.get(id);
            if (row) {
              row.last_notified_at = timestamp;
            }
          }

          // Handle DELETE FROM cached_places
          if (sql.includes('DELETE FROM cached_places')) {
            db.data.clear();
          }
        }),

        withTransactionAsync: jest.fn(async (fn) => {
          return fn();
        }),
      };
    }),
  };
});
