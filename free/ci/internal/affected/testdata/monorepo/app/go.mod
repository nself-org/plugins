module example.test/app

go 1.22

require example.test/mid v0.0.0

require example.test/leaf v0.0.0 // indirect

replace (
	example.test/leaf => ../leaf
	example.test/mid => ../mid
)
