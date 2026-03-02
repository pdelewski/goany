package anyllm

// Chat format constants
const ChatFormatNone int = 0
const ChatFormatChatML int = 1
const ChatFormatMistral int = 2

// containsStr checks if haystack contains needle using byte-level comparison
// Returns 1 if found, 0 if not found
func containsStr(haystack string, needle string) int {
	hLen := len(haystack)
	nLen := len(needle)
	if nLen == 0 {
		return 1
	}
	if nLen > hLen {
		return 0
	}
	i := 0
	limit := hLen - nLen + 1
	for i < limit {
		matched := 1
		j := 0
		for j < nLen {
			if int(haystack[i+j]) != int(needle[j]) {
				matched = 0
				j = nLen // break
			} else {
				j = j + 1
			}
		}
		if matched == 1 {
			return 1
		}
		i = i + 1
	}
	return 0
}

// DetectChatFormat detects chat format based on model architecture
func DetectChatFormat(cfg ModelConfig) int {
	if cfg.Architecture == "llama" {
		return ChatFormatChatML
	} else if cfg.Architecture == "mistral" {
		return ChatFormatMistral
	}
	return ChatFormatNone
}

// DetectChatFormatFromFile detects chat format from GGUF metadata
// Checks tokenizer.chat_template first, falls back to architecture
func DetectChatFormatFromFile(file GGUFFile, cfg ModelConfig) int {
	tmpl := GetMetadataString(file, "tokenizer.chat_template", "")
	if tmpl != "" {
		if containsStr(tmpl, "im_start") == 1 {
			return ChatFormatChatML
		} else if containsStr(tmpl, "[INST]") == 1 {
			return ChatFormatMistral
		}
	}
	return DetectChatFormat(cfg)
}

// FormatChatML formats a prompt using ChatML template
func FormatChatML(prompt string) string {
	result := "<|im_start|>user\n"
	result += prompt
	result += "<|im_end|>\n<|im_start|>assistant\n"
	return result
}

// FormatMistral formats a prompt using Mistral instruct template
// Note: no <s> prefix since Encode() already prepends BOS
func FormatMistral(prompt string) string {
	result := "[INST] "
	result += prompt
	result += " [/INST]"
	return result
}

// ApplyChatTemplate applies the appropriate chat template to a prompt
func ApplyChatTemplate(prompt string, chatFormat int) string {
	if chatFormat == ChatFormatChatML {
		return FormatChatML(prompt)
	} else if chatFormat == ChatFormatMistral {
		return FormatMistral(prompt)
	}
	return prompt
}
