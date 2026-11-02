package main

func runMemoryCommand(args []string) error {
	if len(args) < 1 {
		return errBotUsage
	}
	m := map[string]any{}
	rest := args[1:]
	switch args[0] {
	case "reset-pack":
		return memoryToolCall("memory_reset_pack", m, rest)
	case "reset-compare":
		return memoryToolCall("memory_reset_compare", m, rest)
	default:
		return errBotUsage
	}
}

func memoryToolCall(tool string, base map[string]any, args []string) error {
	for i := 0; i < len(args); i++ {
		if args[i] == "--project" && i+1 < len(args) {
			base["project"] = args[i+1]
			i++
		}
	}
	return usageToolCall(tool, base)
}
