import { isOnline } from "../network";
import * as Network from "expo-network";

jest.mock("expo-network");

const mockedGetNetworkStateAsync = Network.getNetworkStateAsync as jest.MockedFunction<
  typeof Network.getNetworkStateAsync
>;

describe("isOnline", () => {
  it("is true when isInternetReachable is true", async () => {
    mockedGetNetworkStateAsync.mockResolvedValue({
      isConnected: true,
      isInternetReachable: true,
    } as Awaited<ReturnType<typeof Network.getNetworkStateAsync>>);
    expect(await isOnline()).toBe(true);
  });

  it("is false when isInternetReachable is false even if isConnected is true", async () => {
    mockedGetNetworkStateAsync.mockResolvedValue({
      isConnected: true,
      isInternetReachable: false,
    } as Awaited<ReturnType<typeof Network.getNetworkStateAsync>>);
    expect(await isOnline()).toBe(false);
  });

  it("falls back to isConnected when isInternetReachable is undefined", async () => {
    mockedGetNetworkStateAsync.mockResolvedValue({
      isConnected: true,
      isInternetReachable: undefined,
    } as Awaited<ReturnType<typeof Network.getNetworkStateAsync>>);
    expect(await isOnline()).toBe(true);
  });
});
