# Custom-VPS-MCP-Engine: Client Connection Guide

Once deployed and live at **`mcp.my-vps-domain.com`**, the server speaks the **MCP HTTP + SSE transport**. Every request must carry the shared secret (`MCP_SECRET_TOKEN`).

> **Ops prerequisite:** Point the `mcp.my-vps-domain.com` DNS A-record at the VPS, and ensure the Nginx `:443` server has a TLS cert valid for that hostname.

## Endpoints & Headers

| Component | Value |
| :--- | :--- |
| **SSE stream (open session)** | `GET https://mcp.my-vps-domain.com/sse` |
| **Message channel** | `POST https://mcp.my-vps-domain.com/message?sessionid=<id>` |
| **Auth (preferred)** | `Authorization: Bearer <MCP_SECRET_TOKEN>` |
| **Auth (fallback)** | append `?token=<MCP_SECRET_TOKEN>` to the URL |

**Note:** The `sessionid` is not something you invent. After you `GET /sse` with valid auth, the server's first SSE frame is an `endpoint` event telling you the exact `/message?sessionid=...` URL to POST to.

---

## 1. Smoke Test with cURL

Use this to verify your server is reachable and authenticating correctly.

```bash
# Open the stream (blocks, printing SSE events; the first is the 'endpoint' event)
curl -N \
  -H "Authorization: Bearer $MCP_SECRET_TOKEN" \
  [https://mcp.my-vps-domain.com/sse](https://mcp.my-vps-domain.com/sse)

• A missing or wrong token returns 401 Unauthorized.
• A healthy connection stays open and streams events.
2. Claude Desktop Integration (via mcp-remote)
Claude Desktop launches MCP servers over stdio, so you must use the mcp-remote adapter to bridge to our remote SSE endpoint.
Add the following to your claude_desktop_config.json:
{
  "mcpServers": {
    "custom-vps-mcp-engine": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "[https://mcp.my-vps-domain.com/sse](https://mcp.my-vps-domain.com/sse)",
        "--header",
        "Authorization: Bearer YOUR_MCP_SECRET_TOKEN"
      ]
    }
  }
}

If a specific client or bridge cannot send headers on the handshake, use the query-parameter fallback instead:
"https://mcp.my-vps-domain.com/sse?token=YOUR_MCP_SECRET_TOKEN"
3. Local Ollama or Custom Python Script
You can point any MCP-compatible SSE client at the base URL with the bearer header. Here is pseudo-code using a generic Python MCP client library:
from mcp.client.sse import sse_client
from mcp import ClientSession

async with sse_client(
    url="[https://mcp.my-vps-domain.com/sse](https://mcp.my-vps-domain.com/sse)",
    headers={"Authorization": "Bearer YOUR_MCP_SECRET_TOKEN"},
) as (read, write):
    async with ClientSession(read, write) as session:
        await session.initialize()
        
        # Discover available tools
        tools = await session.list_tools()
        # Returns: system_health, system_time
        
        # Execute a tool
        result = await session.call_tool(
            "system_time",
            {"timezone": "America/New_York"}
        )
        print(result.content[0].text)

An LLM behind Ollama or Claude will now see two tools: system_health (no args) and system_time (optional timezone). If the LLM passes an invalid timezone, it receives a self-healing IsError message listing valid examples and can immediately retry correctly.
