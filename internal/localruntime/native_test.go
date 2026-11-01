package localruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeQuestionIdentityAnswersAndSecretRetention(t *testing.T) {
	decode := func(turn string) NativeRequest {
		t.Helper()
		params, _ := json.Marshal(map[string]any{"threadId": "thread", "turnId": turn, "questions": []NativeQuestion{{ID: "choice", Options: []NativeOption{{Label: "Yes"}}}, {ID: "secret", IsSecret: true}}})
		request, err := decodeNativeQuestion(nativeMessage{ID: json.RawMessage(`7`), Method: "item/tool/requestUserInput", Params: params})
		if err != nil {
			t.Fatal(err)
		}
		return request
	}
	request := decode("one")
	if request.ID == decode("two").ID {
		t.Fatal("RPC IDs reused across turns collided")
	}
	reply := NativeReply{RequestID: request.ID, OperationID: "answer", Answers: map[string]NativeAnswer{"choice": {Answers: []string{"Yes"}}, "secret": {Answers: []string{"private-value"}}}}
	result, err := NativeReplyResult(request, reply)
	if err != nil {
		t.Fatal(err)
	}
	retainNativeReply(&request, result, "native client")
	data, _ := json.Marshal(request)
	if strings.Contains(string(data), "private-value") || request.Answers["secret"].Answers[0] != "[redacted]" {
		t.Fatal("declared secret retained")
	}
	reply.Answers["choice"] = NativeAnswer{Answers: []string{"Unoffered"}}
	if _, err = NativeReplyResult(request, reply); err == nil {
		t.Fatal("unoffered answer accepted")
	}
	delete(reply.Answers, "secret")
	if _, err = NativeReplyResult(request, reply); err == nil {
		t.Fatal("incomplete answer accepted")
	}
	request.State = "resolved"
	if _, err = NativeReplyResult(request, reply); err == nil {
		t.Fatal("resolved request accepted")
	}
	for _, raw := range []string{`null`, `true`, `1.5`, `9223372036854775808`, `{}`, `""`} {
		if nativeID(json.RawMessage(raw)) != "" {
			t.Fatal("invalid RPC ID", raw)
		}
	}
}

func TestNativePermissionOnlyOffersDeclaredSimpleDecisions(t *testing.T) {
	request, err := decodeNativeQuestion(nativeMessage{ID: json.RawMessage(`"approval"`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"t","turnId":"u","availableDecisions":["decline",{"acceptWithExecpolicyAmendment":{}}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Decisions) != 1 || request.Decisions[0] != "decline" {
		t.Fatal("invented native choices", request.Decisions)
	}
	if _, err = NativeReplyResult(request, NativeReply{RequestID: request.ID, OperationID: "x", Decision: "accept"}); err == nil {
		t.Fatal("unoffered allow accepted")
	}
}
