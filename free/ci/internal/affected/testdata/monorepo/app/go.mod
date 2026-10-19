module example.test/app

go 1.22

require example.test/mid v0.0.0

replace (
	example.test/mid => ../mid
	example.test/leaf => ../leaf
)
