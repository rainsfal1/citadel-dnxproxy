package policy

import "testing"

func TestDeviceAssignmentUsedInDecision(t *testing.T) {
	data := Data{
		Version: CurrentSchemaVersion,
		Users: map[string]User{
			"u1": {ID: "u1", Name: "Kid", DailyBudgetMinutes: 30},
		},
		Devices: map[string]Device{
			"d1": {ID: "d1", Name: "tablet", IP: "10.0.0.5", MAC: "aa:bb:cc:00:00:01", UserID: "u1"},
		},
	}
	rt := Compile(data)
	e := Evaluator{Runtime: rt}
	dev, ok := e.MatchDevice("10.0.0.5")
	if !ok || dev.UserID != "u1" {
		t.Fatalf("device lookup failed; got user %s", dev.UserID)
	}
	user, ok := e.MatchUser(dev)
	if !ok || user.ID != "u1" {
		t.Fatalf("user not resolved; got %s", user.ID)
	}
}
