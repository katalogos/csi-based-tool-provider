/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package toolprovider

import (
	"os/exec"
	"strings"

	"golang.org/x/net/context"
)

const buildahPath = "/bin/buildah"

func runCmd(ctx context.Context, execPath string, args ...string) (string, error) {
	logger := contextLogger(ctx)
	cmd := exec.Command(execPath, args...)
	logger.LeveledInfof(levelRunCommand, "Executing command: %s\n", cmd.String())
	output, execErr := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output[:]))
	logger.LeveledInfof(levelRunCommand, "    => Output = %s\n", result)
	return result, execErr
}
