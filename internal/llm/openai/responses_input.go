package openai

import (
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	rs "github.com/openai/openai-go/v3/responses"

	"manifold/internal/llm"
)

func adaptResponsesTools(schemas []llm.ToolSchema) []rs.ToolUnionParam {
	out := make([]rs.ToolUnionParam, 0, len(schemas))
	for _, s := range schemas {
		if isNativeWebSearchSchema(s.Name) {
			out = append(out, rs.ToolParamOfWebSearch(rs.WebSearchToolTypeWebSearch))
			continue
		}
		params := s.Parameters
		if params != nil {
			params = ensureStrictJSONSchema(params).(map[string]any)
		}
		fn := rs.FunctionToolParam{
			Name:        s.Name,
			Parameters:  params,
			Strict:      sdk.Bool(false),
			Description: sdk.String(s.Description),
		}
		out = append(out, rs.ToolUnionParam{OfFunction: &fn})
	}
	return out
}

func adaptResponsesInputWithLimit(msgs []llm.Message, toolOutputMaxChars int) (items rs.ResponseInputParam, instructions string) {
	validToolCallIDs := validResponsesToolCallIDs(msgs)
	items = make([]rs.ResponseInputItemUnionParam, 0, len(msgs))
	var sys []string
	for _, m := range msgs {
		if m.Compaction != nil {
			items = append(items, responseCompactionItemParam(*m.Compaction))
			continue
		}
		items, sys = appendResponsesInputMessage(items, sys, m, false, nil, validToolCallIDs, toolOutputMaxChars)
	}
	if len(sys) > 0 {
		instructions = strings.Join(sys, "\n\n")
	}
	return items, instructions
}

func adaptResponsesInputWithImagesAndLimit(msgs []llm.Message, images []ImageAttachment, toolOutputMaxChars int) (items rs.ResponseInputParam, instructions string) {
	if len(images) == 0 {
		return adaptResponsesInputWithLimit(msgs, toolOutputMaxChars)
	}
	validToolCallIDs := validResponsesToolCallIDs(msgs)
	lastUserIdx := lastResponsesUserIndex(msgs)
	items = make([]rs.ResponseInputItemUnionParam, 0, len(msgs))
	var sys []string
	for i, m := range msgs {
		if m.Compaction != nil {
			items = append(items, responseCompactionItemParam(*m.Compaction))
			continue
		}
		items, sys = appendResponsesInputMessage(items, sys, m, i == lastUserIdx, images, validToolCallIDs, toolOutputMaxChars)
	}
	if len(sys) > 0 {
		instructions = strings.Join(sys, "\n\n")
	}
	return items, instructions
}

func validResponsesToolCallIDs(msgs []llm.Message) map[string]struct{} {
	validToolCallIDs := make(map[string]struct{}, 8)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			if strings.TrimSpace(tc.ID) != "" {
				validToolCallIDs[tc.ID] = struct{}{}
			}
		}
	}
	return validToolCallIDs
}

func lastResponsesUserIndex(msgs []llm.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return i
		}
	}
	return -1
}

func appendResponsesInputMessage(
	items []rs.ResponseInputItemUnionParam,
	sys []string,
	m llm.Message,
	withImages bool,
	images []ImageAttachment,
	validToolCallIDs map[string]struct{},
	toolOutputMaxChars int,
) ([]rs.ResponseInputItemUnionParam, []string) {
	switch m.Role {
	case "system":
		if strings.TrimSpace(m.Content) != "" {
			sys = append(sys, m.Content)
		}
	case "user":
		items = append(items, responsesUserInputMessage(m.Content, withImages, images))
	case "assistant":
		items = appendResponsesAssistantCalls(items, m)
	case "tool":
		items = appendResponsesToolOutput(items, m, validToolCallIDs, toolOutputMaxChars)
	}
	return items, sys
}

func responsesUserInputMessage(content string, withImages bool, images []ImageAttachment) rs.ResponseInputItemUnionParam {
	content = strings.TrimSpace(content)
	if content == "" {
		content = " "
	}
	parts := rs.ResponseInputMessageContentListParam{rs.ResponseInputContentParamOfInputText(content)}
	if withImages {
		parts = responsesImageContentParts(content, images)
	}
	return rs.ResponseInputItemUnionParam{OfInputMessage: &rs.ResponseInputItemMessageParam{
		Content: parts,
		Role:    "user",
	}}
}

func responsesImageContentParts(content string, images []ImageAttachment) rs.ResponseInputMessageContentListParam {
	parts := make(rs.ResponseInputMessageContentListParam, 0, 1+len(images))
	if strings.TrimSpace(content) != "" {
		parts = append(parts, rs.ResponseInputContentParamOfInputText(content))
	}
	for _, img := range images {
		if strings.TrimSpace(img.MimeType) == "" || strings.TrimSpace(img.Base64Data) == "" {
			continue
		}
		parts = append(parts, responseInputImageContentParam("data:"+img.MimeType+";base64,"+img.Base64Data))
	}
	if len(parts) == 0 {
		parts = append(parts, rs.ResponseInputContentParamOfInputText(" "))
	}
	return parts
}

func appendResponsesAssistantCalls(items []rs.ResponseInputItemUnionParam, m llm.Message) []rs.ResponseInputItemUnionParam {
	for _, tc := range m.ToolCalls {
		items = append(items, rs.ResponseInputItemParamOfFunctionCall(string(tc.Args), tc.ID, tc.Name))
	}
	return items
}

func appendResponsesToolOutput(
	items []rs.ResponseInputItemUnionParam,
	m llm.Message,
	validToolCallIDs map[string]struct{},
	toolOutputMaxChars int,
) []rs.ResponseInputItemUnionParam {
	toolID := strings.TrimSpace(m.ToolID)
	if toolID == "" {
		return items
	}
	if _, ok := validToolCallIDs[toolID]; !ok {
		return items
	}
	out := boundedResponsesToolOutputWithLimit(m.Content, toolOutputMaxChars)
	return append(items, rs.ResponseInputItemParamOfFunctionCallOutput(toolID, out))
}

func responseInputImageContentParam(dataURL string) rs.ResponseInputContentUnionParam {
	return rs.ResponseInputContentUnionParam{
		OfInputImage: &rs.ResponseInputImageParam{
			Detail:   rs.ResponseInputImageDetailAuto,
			ImageURL: param.NewOpt(dataURL),
		},
	}
}

func responseCompactionItemParam(item llm.CompactionItem) rs.ResponseInputItemUnionParam {
	compaction := rs.ResponseCompactionItemParam{EncryptedContent: item.EncryptedContent}
	if strings.TrimSpace(item.ID) != "" {
		compaction.ID = param.NewOpt(item.ID)
	}
	return rs.ResponseInputItemUnionParam{OfCompaction: &compaction}
}
