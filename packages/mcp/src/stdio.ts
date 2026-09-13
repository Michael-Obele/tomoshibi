#!/usr/bin/env node
import { StdioTransport } from "@tmcp/transport-stdio";
import { createServer } from "./server.js";

const server = createServer();
const transport = new StdioTransport(server);
await transport.listen();
