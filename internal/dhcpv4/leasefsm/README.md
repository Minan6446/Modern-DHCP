# DHCPv4 leasefsm

职责：维护 DHCPv4 租约状态机及合法状态转移规则，供 handler/lease 层调用，避免状态逻辑散落。
