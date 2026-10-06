package domain

import "testing"

func TestRepairRecordValidatesAndClassifiesStatuses(t *testing.T) {
	record := RepairRecord{ID: "r1", Signature: "sig", Repository: "o/r", Parent: "0123456789abcdef0123456789abcdef", Status: RepairRunning}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	if !record.Active() {
		t.Fatal("running is active")
	}
	record.Status = "odd"
	if record.Validate() == nil {
		t.Fatal("unknown status accepted")
	}
	record.Status = RepairInterrupted
	if record.Active() || record.Status.Terminal() {
		t.Fatal("interrupted is neither active nor terminal")
	}
	for _, status := range []RepairStatus{RepairCompleted, RepairBlocked, RepairFailed} {
		if !status.Terminal() || status.Awaiting() {
			t.Fatalf("%s terminal", status)
		}
	}
	for _, status := range []RepairStatus{RepairAwaitingApproval, RepairAwaitingMerge} {
		if !status.Awaiting() || status.Terminal() {
			t.Fatalf("%s awaiting", status)
		}
	}
	if (RepairRecord{}).Validate() == nil {
		t.Fatal("empty record accepted")
	}
	if _, err := NewHostToolIdentity(HostOperationRequestRepair); err != nil {
		t.Fatal(err)
	}
	if _, err := NewHostToolIdentity(HostOperationRepairStatus); err != nil {
		t.Fatal(err)
	}
}
