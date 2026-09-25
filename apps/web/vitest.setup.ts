import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Don DOM sau moi test de test khong anh huong nhau.
afterEach(() => {
  cleanup();
});
