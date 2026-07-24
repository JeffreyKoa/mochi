package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mochi-ai/server/internal/emotion"
	"github.com/mochi-ai/server/internal/models"
	"github.com/mochi-ai/server/internal/tools"
	"github.com/mochi-ai/server/pkg/ai"
)

type toolTurnResult struct {
	messages    []ai.Message
	directReply string
}

func (s *Service) applyToolTurn(
	ctx context.Context,
	messages []ai.Message,
	userMsg string,
	pet *models.Pet,
	userID uint64,
	bond models.BondProfile,
	hint emotion.Hint,
) (toolTurnResult, error) {
	if s.toolsExec == nil || !s.toolsExec.Enabled() {
		return toolTurnResult{messages: messages}, nil
	}
	if hint.NeedsEmpathy || hint.Intent == "vent" {
		return toolTurnResult{messages: messages}, nil
	}

	msgs := appendTimeContext(messages)

	maxTok := s.toolsCfg.ToolTurnMaxTokens
	if maxTok <= 0 {
		maxTok = 256
	}

	resp, err := s.ai.ChatWithTools(ctx, ai.ChatWithToolsRequest{
		Messages:    msgs,
		Tools:       tools.Registry(),
		ToolChoice:  "auto",
		Temperature: 0.2,
		MaxTokens:   maxTok,
	})
	if err != nil {
		return toolTurnResult{messages: messages}, err
	}

	if len(resp.ToolCalls) == 0 {
		if strings.TrimSpace(resp.Content) != "" {
			return toolTurnResult{directReply: strings.TrimSpace(resp.Content)}, nil
		}
		return toolTurnResult{messages: messages}, nil
	}

	exec := tools.ExecContext{
		PetID:   pet.ID,
		UserID:  userID,
		UserMsg: userMsg,
		Bond:    bond,
	}

	out := append([]ai.Message{}, msgs...)
	out = append(out, ai.Message{
		Role:      "assistant",
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	})

	for _, tc := range resp.ToolCalls {
		result := s.toolsExec.Run(ctx, exec, tc)
		out = append(out, ai.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Name:       tc.Function.Name,
			Content:    result,
		})
	}

	out = append(out, ai.Message{
		Role:    "user",
		Content: "请用1-2句口语向主人确认刚才的办事结果，不要说「已为您完成」等服务腔；禁止用括号描述动作，只输出对话。",
	})

	return toolTurnResult{messages: out}, nil
}

func appendTimeContext(messages []ai.Message) []ai.Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]ai.Message, len(messages))
	copy(out, messages)
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	out[0] = ai.Message{
		Role:    messages[0].Role,
		Content: messages[0].Content + fmt.Sprintf("\n\n【当前时间 UTC+8】%s", now.Format(time.RFC3339)),
	}
	return out
}

func streamText(ctx context.Context, text string, onToken func(string)) {
	if onToken == nil {
		return
	}
	for _, r := range text {
		select {
		case <-ctx.Done():
			return
		default:
			onToken(string(r))
		}
	}
}
