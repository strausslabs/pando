import type { Service } from "../types";

export const db: Service = {
  compose: { image: "postgres:16", ports: ["$PORT_db:5432"] },
};
