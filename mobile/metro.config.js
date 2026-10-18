const { getDefaultConfig } = require("expo/metro-config");

const config = getDefaultConfig(__dirname);

// expo-sqlite's web worker imports its wa-sqlite.wasm binary directly;
// without this, Metro's web bundler tries (and fails) to parse it as JS.
config.resolver.assetExts.push("wasm");

module.exports = config;
