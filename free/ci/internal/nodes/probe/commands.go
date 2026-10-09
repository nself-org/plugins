package probe

// Every command is a literal. No node or inventory value enters the remote shell.
const (
	cmdUname     = "uname -srm"
	cmdOSRelease = "cat /etc/os-release"
	cmdNproc     = "nproc"
	cmdDisk      = "df -Pk /"
	cmdTools     = "command -v docker podman runsc tart go node pnpm cargo rustc"
	cmdDocker    = "docker version --format '{{.Server.Version}}'"
	cmdXcode     = "xcodebuild -version"
	cmdXcodePath = "xcode-select -p"
	cmdSwift     = "swift --version"
	cmdPython    = "python3 --version"
	cmdTart      = "tart --version"
	cmdNvidia    = "nvidia-smi -L"
	cmdDisplays  = "system_profiler SPDisplaysDataType -json"
)

var linuxCommands = []string{cmdUname, cmdOSRelease, cmdNproc, cmdDisk, cmdTools, cmdDocker, cmdPython, cmdNvidia}
var darwinCommands = []string{cmdUname, cmdNproc, cmdDisk, cmdTools, cmdDocker, cmdXcode, cmdXcodePath, cmdSwift, cmdPython, cmdTart, cmdDisplays}

func fixedCommand(command string) bool {
	for _, candidate := range linuxCommands {
		if command == candidate {
			return true
		}
	}
	for _, candidate := range darwinCommands {
		if command == candidate {
			return true
		}
	}
	return false
}
