import { listFeaturedItineraries, ItinerariesApiError } from "../ItinerariesRepository";

describe("listFeaturedItineraries", () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    jest.clearAllMocks();
  });

  it("maps the wire response without sending an Authorization header -- this route is public", async () => {
    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => [
        {
          id: "it-1",
          title: "Roteiro do Rio Colonial",
          total_minutes: 45,
          place_count: 2,
          stops: [
            { kind: "place", place_id: "place-1", label: "Paço Imperial", time_on_site_minutes: 20, walk_to_next_minutes: 5 },
            { kind: "suggestion", label: "Pausa para almoço", time_on_site_minutes: 0, walk_to_next_minutes: 0 },
          ],
        },
      ],
    });
    globalThis.fetch = mockFetch as unknown as typeof fetch;

    const result = await listFeaturedItineraries();

    expect(result).toEqual([
      {
        id: "it-1",
        title: "Roteiro do Rio Colonial",
        totalMinutes: 45,
        placeCount: 2,
        stops: [
          { kind: "place", placeId: "place-1", label: "Paço Imperial", timeOnSiteMinutes: 20, walkToNextMinutes: 5 },
          { kind: "suggestion", label: "Pausa para almoço", timeOnSiteMinutes: 0, walkToNextMinutes: 0 },
        ],
      },
    ]);
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const [, options] = mockFetch.mock.calls[0];
    expect(options?.headers).toBeUndefined();
  });

  it("throws ItinerariesApiError on a non-ok response", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 500,
      json: async () => ({ error: "internal" }),
    }) as unknown as typeof fetch;

    await expect(listFeaturedItineraries()).rejects.toBeInstanceOf(ItinerariesApiError);
  });

  it("throws ItinerariesApiError rather than an unhandled parse error on a malformed body", async () => {
    globalThis.fetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => {
        throw new SyntaxError("Unexpected token <");
      },
    }) as unknown as typeof fetch;

    await expect(listFeaturedItineraries()).rejects.toBeInstanceOf(ItinerariesApiError);
  });
});
