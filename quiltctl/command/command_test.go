package command

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	clientMock "github.com/NetSys/quilt/api/client/mocks"
	"github.com/NetSys/quilt/db"
	"github.com/NetSys/quilt/quiltctl/testutils"
)

func TestMachineFlags(t *testing.T) {
	t.Parallel()

	expHost := "IP"

	machineCmd := NewMachineCommand()
	err := parseHelper(machineCmd, []string{"-H", expHost})

	assert.NoError(t, err)
	assert.Equal(t, expHost, machineCmd.common.host)
}

func TestMachineOutput(t *testing.T) {
	t.Parallel()

	res := machinesStr([]db.Machine{{
		ID:       1,
		Role:     db.Master,
		Provider: "Amazon",
		Region:   "us-west-1",
		Size:     "m4.large",
		PublicIP: "8.8.8.8",
	}})

	exp := "Machine-1{Master, Amazon us-west-1 m4.large, PublicIP=8.8.8.8}\n"

	assert.Equal(t, exp, res)
}

func TestContainerFlags(t *testing.T) {
	t.Parallel()

	expHost := "IP"

	containerCmd := NewContainerCommand()
	err := parseHelper(containerCmd, []string{"-H", expHost})

	assert.NoError(t, err)
	assert.Equal(t, expHost, containerCmd.common.host)
}

func TestContainerOutput(t *testing.T) {
	t.Parallel()

	containers := []db.Container{
		{StitchID: 3, Minion: "3.3.3.3", Image: "image1",
			Command: []string{"cmd", "1"}},
		{StitchID: 1, Minion: "1.1.1.1", Image: "image2",
			Labels: []string{"label1", "label2"}},
		{StitchID: 4, Minion: "1.1.1.1", Image: "image3",
			Command: []string{"cmd"},
			Labels:  []string{"label1"}},
		{StitchID: 7, Minion: "2.2.2.2", Image: "image1",
			Command: []string{"cmd", "3", "4"},
			Labels:  []string{"label1"}},
		{StitchID: 8, Image: "image1"},
	}

	machines := []db.Machine{
		{ID: 5, PrivateIP: "1.1.1.1"},
		{ID: 6, PrivateIP: "2.2.2.2"},
		{ID: 7, PrivateIP: ""},
	}

	var b bytes.Buffer
	writeContainers(&b, machines, containers)
	result := string(b.Bytes())

	/* By replacing space with underscore, we make the spaces explicit and whitespace
	* errors easier to debug. */
	result = strings.Replace(result, " ", "_", -1)

	expected := `ID____MACHINE______IMAGE_____COMMAND______LABELS
__________________________________________
3__________________image1____"cmd_1"______
__________________________________________
1_____Machine-5____image2____""___________label1,_label2
4_____Machine-5____image3____"cmd"________label1
__________________________________________
7_____Machine-6____image1____"cmd_3_4"____label1
__________________________________________
8_____Machine-7____image1____""___________
`
	assert.Equal(t, expected, result)
}

func checkGetParsing(t *testing.T, args []string, expImport string, expErr error) {
	getCmd := &Get{}
	err := parseHelper(getCmd, args)

	assert.Equal(t, expErr, err)
	assert.Equal(t, expImport, getCmd.importPath)
}

func TestGetFlags(t *testing.T) {
	t.Parallel()

	expImport := "spec"
	checkGetParsing(t, []string{"-import", expImport}, expImport, nil)
	checkGetParsing(t, []string{expImport}, expImport, nil)
	checkGetParsing(t, []string{}, "", errors.New("no import specified"))
}

func checkStopParsing(t *testing.T, args []string, expNamespace string, expErr error) {
	stopCmd := NewStopCommand()
	err := parseHelper(stopCmd, args)

	assert.Equal(t, expErr, err)
	assert.Equal(t, expNamespace, stopCmd.namespace)
}

func TestStopFlags(t *testing.T) {
	t.Parallel()

	expNamespace := "namespace"
	checkStopParsing(t, []string{"-namespace", expNamespace}, expNamespace, nil)
	checkStopParsing(t, []string{expNamespace}, expNamespace, nil)
	checkStopParsing(t, []string{}, defaultNamespace, nil)
}

func checkSSHParsing(t *testing.T, args []string, expMachine int,
	expSSHArgs []string, expErr error) {

	sshCmd := NewSSHCommand()
	err := parseHelper(sshCmd, args)

	assert.Equal(t, expErr, err)
	assert.Equal(t, expMachine, sshCmd.targetMachine)
	assert.Equal(t, expSSHArgs, sshCmd.sshArgs)
}

func TestSSHFlags(t *testing.T) {
	t.Parallel()

	checkSSHParsing(t, []string{"1"}, 1, []string{}, nil)
	sshArgs := []string{"-i", "~/.ssh/key"}
	checkSSHParsing(t, append([]string{"1"}, sshArgs...), 1, sshArgs, nil)
	checkSSHParsing(t, []string{}, 0, nil,
		errors.New("must specify a target machine"))
}

func TestStopNamespace(t *testing.T) {
	t.Parallel()

	mockGetter := new(testutils.Getter)
	c := &clientMock.Client{}
	mockGetter.On("Client", mock.Anything).Return(c, nil)

	stopCmd := NewStopCommand()
	stopCmd.clientGetter = mockGetter
	stopCmd.namespace = "namespace"
	stopCmd.Run()
	expStitch := `{"namespace": "namespace"}`

	assert.Equal(t, expStitch, c.DeployArg)
}

func TestSSHCommandCreation(t *testing.T) {
	t.Parallel()

	exp := []string{"ssh", "quilt@host", "-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null", "-i", "~/.ssh/quilt"}
	res := runSSHCommand("host", []string{"-i", "~/.ssh/quilt"})

	assert.Equal(t, exp, res.Args)
}

func parseHelper(cmd SubCommand, args []string) error {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.InstallFlags(flags)
	flags.Parse(args)
	return cmd.Parse(flags.Args())
}
