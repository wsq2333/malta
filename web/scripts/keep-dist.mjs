import { writeFileSync } from "node:fs";

// Keep clean checkouts buildable with Go before the optional frontend build.
writeFileSync(new URL("../dist/.gitkeep", import.meta.url), "");
