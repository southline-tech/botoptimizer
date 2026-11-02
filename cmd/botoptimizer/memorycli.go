package main

func runMemoryCommand(args []string) error {
	if len(args) < 1 {
		return errBotUsage
	}
	m := map[string]any{}
	rest := args[1:]
	switch args[0] {
	case "reset-pack":
		if err := parseResetPackFlags(m, rest); err != nil {
			return err
		}
		return usageToolCall("memory_reset_pack", m)
	case "reset-compare":
		if err := parseResetCompareFlags(m, rest); err != nil {
			return err
		}
		return usageToolCall("memory_reset_compare", m)
	default:
		return errBotUsage
	}
}

func parseResetPackFlags(m map[string]any, args []string) error {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--bot":
			i++
			if i >= len(args) {
				return errBotUsage
			}
			m["bot"] = args[i]
		case "--out":
			i++
			if i >= len(args) {
				return errBotUsage
			}
			m["out_dir"] = args[i]
		case "--save-vault":
			m["save_vault"] = true
		case "--project":
			i++
			if i >= len(args) {
				return errBotUsage
			}
			m["project"] = args[i]
		default:
			return errBotUsage
		}
	}
	if _, ok := m["out_dir"]; !ok {
		return errBotUsage
	}
	return nil
}

func parseResetCompareFlags(m map[string]any, args []string) error {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--before":
			i++
			if i >= len(args) {
				return errBotUsage
			}
			m["before_path"] = args[i]
		case "--bot":
			i++
			if i >= len(args) {
				return errBotUsage
			}
			m["bot"] = args[i]
		case "--project":
			i++
			if i >= len(args) {
				return errBotUsage
			}
			m["project"] = args[i]
		default:
			return errBotUsage
		}
	}
	return nil
}
