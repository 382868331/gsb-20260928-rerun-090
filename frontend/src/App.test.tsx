import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import App from "./App";

type Handler = (url: string, init?: RequestInit) => Promise<{ status: number; body: unknown }>;

let handler: Handler;

function installFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const res = await handler(url, init);
      return {
        ok: res.status >= 200 && res.status < 300,
        status: res.status,
        json: async () => res.body,
      };
    }),
  );
}

describe("story page", () => {
  beforeEach(() => {
    installFetch();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders only allowed choices and keeps server authoritative on submit", async () => {
    let gateViewChoices = [
      { id: "beg", text: "Beg the guard", target: "gate_rejected", allowed: true },
    ];
    handler = async (url, init) => {
	  if (url.endsWith("/choices")) {
		const payload = JSON.parse(String(init?.body));
		if (payload.choiceId === "pay") {
		  return { status: 409, body: { error: "choice condition is not satisfied" } };
		}
		return {
		  status: 200,
		  body: {
			instance: {
			  id: "i1", storyId: "hero-road", version: "1", defHash: "h",
			  current: "gate_rejected", facts: { gold: 0 },
			  claimedRewards: ["gate:beg"], requestIds: {}, log: [],
			  createdAt: "", updatedAt: "",
			},
			result: {
			  node: { id: "gate_rejected", type: "choice", text: "Rejected", choices: [] },
			  availableChoices: [
				{ id: "back_forest", text: "Head to forest", target: "forest", allowed: true },
			  ],
			  choiceId: "beg", appliedEffects: [], rewards: [], replayed: false,
			},
		  },
		};
	  }
	  if (url.endsWith("/view")) {
		return {
		  status: 200,
		  body: {
			instance: {
			  id: "i1", storyId: "hero-road", version: "1", defHash: "h",
			  current: "gate", facts: { gold: 0 },
			  claimedRewards: [], requestIds: {}, log: [],
			  createdAt: "", updatedAt: "",
			},
			view: {
			  node: { id: "gate", type: "choice", text: "Toll gate", choices: [] },
			  availableChoices: gateViewChoices,
			},
		  },
		};
	  }
	if (url.endsWith("/api/instances")) {
        return {
          status: 200,
          body: {
            instances: [
              {
                id: "i1",
                storyId: "hero-road",
                version: "1",
                defHash: "h",
                current: "gate",
                facts: { gold: 0 },
                claimedRewards: [],
                requestIds: {},
                log: [],
                createdAt: "",
                updatedAt: "",
              },
            ],
          },
        };
      }
      return { status: 404, body: {} };
    };

    render(<App />);

    await waitFor(() => expect(screen.getByText("Toll gate")).toBeTruthy());
    // Only "beg" exists; "pay" must not be rendered at all.
    expect(screen.queryByText("Pay the 10 gold toll")).toBeNull();
    expect(screen.getByText("Beg the guard")).toBeTruthy();

    fireEvent.click(screen.getByText("Beg the guard"));
    await waitFor(() => expect(screen.getByText("Rejected")).toBeTruthy());

    // Simulate the view gaining "pay" after facts changed elsewhere; a forged
    // submit still fails server-side and surfaces an error notice.
    gateViewChoices = [
      { id: "pay", text: "Pay the toll", target: "city", allowed: true },
      { id: "beg", text: "Beg the guard", target: "gate_rejected", allowed: true },
    ];
    // Navigate back to gate via reloading i1 view: emulate by choosing a
    // hypothetical back action is unnecessary; directly check error path by
    // calling fetch with pay against current state through a fresh handler:
    // open another instance snapshot by clicking its sidebar entry.
    fireEvent.click(screen.getAllByText(/i1/)[0]);
    await waitFor(() => expect(screen.getByText("Pay the toll")).toBeTruthy());
    fireEvent.click(screen.getByText("Pay the toll"));
    await waitFor(() =>
      expect(screen.getByText(/choice condition is not satisfied/)).toBeTruthy(),
    );
  });

  it("marks repeated submissions as replayed with no new rewards", async () => {
    let calls = 0;
    handler = async (url) => {
      if (url.endsWith("/api/instances") && !url.includes("/choices")) {
        return {
          status: 200,
          body: {
            instances: [
              {
                id: "i2",
                storyId: "hero-road",
                version: "1",
                defHash: "h",
                current: "forest",
                facts: { gold: 10, tookGold: true },
                claimedRewards: [],
                requestIds: { k: { node: "forest", choice: "take_gold" } },
                log: [],
                createdAt: "",
                updatedAt: "",
              },
            ],
          },
        };
      }
      if (url.endsWith("/view")) {
        return {
          status: 200,
          body: {
            instance: {
              id: "i2",
              storyId: "hero-road",
              version: "1",
              defHash: "h",
              current: "forest",
              facts: { gold: 10 },
              claimedRewards: [],
              requestIds: {},
              log: [],
              createdAt: "",
              updatedAt: "",
            },
            view: {
              node: { id: "forest", type: "choice", text: "Forest", choices: [] },
              availableChoices: [
                { id: "take_gold", text: "Take pouch", target: "forest_after", allowed: true },
              ],
            },
          },
        };
      }
      if (url.endsWith("/choices")) {
        calls++;
        return {
          status: 200,
          body: {
            instance: {
              id: "i2",
              storyId: "hero-road",
              version: "1",
              defHash: "h",
              current: "forest_after",
              facts: { gold: 10 },
              claimedRewards: ["forest:take_gold"],
              requestIds: {},
              log: [],
              createdAt: "",
              updatedAt: "",
            },
            result: {
              node: { id: "forest_after", type: "choice", text: "After", choices: [] },
              availableChoices: [],
              choiceId: "take_gold",
              appliedEffects: [],
              rewards: [],
              replayed: true,
            },
          },
        };
      }
      return { status: 404, body: {} };
    };

    render(<App />);
    await waitFor(() => expect(screen.getByText("Forest")).toBeTruthy());
    fireEvent.click(screen.getByText("Take pouch"));
    await waitFor(() =>
      expect(screen.getByText("重复提交已识别：未重复发放奖励。")).toBeTruthy(),
    );
    expect(calls).toBe(1);
  });
});
