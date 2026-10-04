package application

import (
	"encoding/json"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
)

func hostHistory(messages []root.Message, arguments root.JSONValue) (any, error) {
	args, err := decodeHostArguments(arguments, "message_index", "offset_bytes", "limit_bytes")
	if err != nil {
		return nil, err
	}
	index, err := hostInteger(args, "message_index", -1)
	if err != nil || index < 0 || index >= len(messages) {
		return nil, errors.New("message_index is outside the persisted session")
	}
	// History recovers what an earlier turn left out of context. Re-reading a
	// result of the turn in progress refills the context its excerpt saved:
	// seen on 2 Oct 2026, paging a 63 KB catalogue back aborted the turn.
	if messages[index].Role == root.RoleTool && index > lastUserMessage(messages) {
		return nil, errors.New("message_index is a result of the current turn; its excerpt is already in your context. Repeat the original call with a narrower query, filter or page instead")
	}
	offset, err := hostInteger(args, "offset_bytes", 0)
	if err != nil || offset < 0 {
		return nil, errors.New("offset_bytes must be a nonnegative integer")
	}
	limit, err := hostInteger(args, "limit_bytes", 4096)
	if err != nil || limit < 1 || limit > MaxHistoryReadBytes {
		return nil, errors.New("limit_bytes must be between 1 and 32768")
	}
	data, err := json.Marshal(messages[index])
	if err != nil {
		return nil, err
	}
	if offset > len(data) || !hostUTF8Boundary(data, offset) {
		return nil, errors.New("offset_bytes is outside the message or splits a UTF-8 character")
	}
	end := offset + min(limit, len(data)-offset)
	for !hostUTF8Boundary(data, end) {
		end--
	}
	if end == offset && offset < len(data) {
		return nil, errors.New("limit_bytes is too small for the next UTF-8 character")
	}
	for {
		result := map[string]any{"message_index": index, "role": messages[index].Role, "offset_bytes": offset, "next_offset_bytes": end, "total_bytes": len(data), "has_more": end < len(data), "text": string(data[offset:end])}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		if contentJSONBytes(string(encoded)) <= MaxHistoryReadBytes-256 {
			return result, nil
		}
		end = offset + (end-offset)/2
		for !hostUTF8Boundary(data, end) {
			end--
		}
		if end == offset {
			return nil, errors.New("history page cannot fit the host result budget")
		}
	}
}

func hostUTF8Boundary(data []byte, offset int) bool {
	return offset == len(data) || offset == 0 || data[offset]&0xc0 != 0x80
}

func lastUserMessage(messages []root.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == root.RoleUser {
			return i
		}
	}
	return -1
}
