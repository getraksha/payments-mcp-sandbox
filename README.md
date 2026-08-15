# payments-mcp-sandbox
A synthetic, enterprise-style MCP server for demonstrating shared payment operations governed by AI agents.

The server exposes simulated payment tools such as:

* add_recipient
* list_recipients
* transfer_money
* list_transactions
* check_transfer_status

The MCP server represents a shared enterprise service and intentionally does not implement end-user authentication or authorization. Access to tools and actions can be governed externally by systems such as AGP.

All recipients, accounts, transactions, and money transfers are synthetic. **No real financial transactions are performed.**
