// Mock expo-sqlite for Jest testing with an in-memory database
jest.mock('expo-sqlite', () => {
  // Simple in-memory SQLite implementation for testing
  const databases = new Map();

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
              // Upsert: preserve columns NOT in the UPDATE SET clause
              const existing = db.data.get(id);
              const merged = { ...existing };

              // Parse which columns are in the ON CONFLICT ... DO UPDATE SET clause
              const updateMatch = sql.match(/ON CONFLICT\s*\([^)]+\)\s*DO UPDATE SET\s+(.+?)(?:;|$)/i);
              const columnsToUpdate = new Set();
              if (updateMatch) {
                const setClause = updateMatch[1];
                const assignments = setClause.split(',').map(s => s.trim());
                for (const assignment of assignments) {
                  const columnMatch = assignment.match(/^(\w+)\s*=/);
                  if (columnMatch) {
                    columnsToUpdate.add(columnMatch[1]);
                  }
                }
              }

              // Apply updates only to columns explicitly in the SET clause
              if (columnsToUpdate.has('name')) merged.name = newRow.name;
              if (columnsToUpdate.has('category')) merged.category = newRow.category;
              if (columnsToUpdate.has('lat')) merged.lat = newRow.lat;
              if (columnsToUpdate.has('lon')) merged.lon = newRow.lon;
              if (columnsToUpdate.has('body')) merged.body = newRow.body;
              if (columnsToUpdate.has('audio_local_uri')) merged.audio_local_uri = newRow.audio_local_uri;
              if (columnsToUpdate.has('last_notified_at')) merged.last_notified_at = newRow.last_notified_at;

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
