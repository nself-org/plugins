package invariants

type Invariant struct{ ID, Statement, Owner, Test string }

var Catalog = []Invariant{
	{"I01", "untrusted only on accepting runner", "P7-TRUST-01", "TestInvariant_I01"},
	{"I02", "platform requirement honored", "P7-SCHED-01", "TestInvariant_I02"},
	{"I03", "private jobs never hosted", "P7-TRUST-01", "TestInvariant_I03"},
	{"I04", "release secrets require isolation and trust", "P7-TRUST-01", "TestInvariant_I04"},
	{"I05", "untrusted never receives secrets", "P7-TRUST-02", "TestInvariant_I05"},
	{"I06", "secret values never appear in output", "P7-TRUST-19", "TestInvariant_I06"},
	{"I07", "untrusted never gets LAN", "P7-TRUST-08", "TestInvariant_I07"},
	{"I08", "attempts cannot observe predecessors", "P7-TRUST-09", "TestInvariant_I08"},
	{"I09", "policy never loosens parent", "P7-TRUST-01", "TestInvariant_I09"},
	{"I10", "retry never lowers isolation", "P7-TRUST-01", "TestInvariant_I10"},
	{"I11", "untrusted cannot write trusted cache", "P7-TRUST-10", "TestInvariant_I11"},
	{"I12", "evidence binds revision and inputs", "P7-TRUST-11", "TestInvariant_I12"},
	{"I13", "stale evidence cannot pass gate", "P7-TRUST-11", "TestInvariant_I13"},
	{"I14", "runner capacity enforced", "P7-SCHED-04", "TestInvariant_I14"},
	{"I15", "production boundary enforced", "P7-TRUST-14", "TestInvariant_I15"},
	{"I16", "revision cannot authorize policy", "P7-TRUST-01", "TestInvariant_I16"},
	{"I17", "job UID distinct from agent", "P7-TRUST-09", "TestInvariant_I17"},
	{"I18", "untrusted cannot reach deploy network", "P7-TRUST-08", "TestInvariant_I18"},
	{"I19", "spend needs operator authorization", "P7-SCHED-01", "TestInvariant_I19"},
}
