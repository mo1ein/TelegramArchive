package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Dynamic values travel as process arguments, never as script source or
// shell code. NUL delimiters preserve filenames containing commas or newlines.
const macPathDialogScript = `ObjC.import('AppKit');
function run(argv) {
    var kind = argv[0];
    var panel = kind === 'save' ? $.NSSavePanel.savePanel : $.NSOpenPanel.openPanel;
    panel.title = 'Telegram Archive';
    panel.message = argv[1];
    panel.directoryURL = $.NSURL.fileURLWithPath(argv[2]);
    panel.canCreateDirectories = true;
    if (kind === 'save') {
        panel.nameFieldStringValue = argv[3];
    } else {
        panel.canChooseFiles = kind !== 'directory';
        panel.canChooseDirectories = kind === 'directory' || kind === 'any';
        panel.allowsMultipleSelection = argv[4] === 'true';
    }
    var app = $.NSApplication.sharedApplication;
    app.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
    app.activateIgnoringOtherApps(true);
    if (Number(panel.runModal) !== Number($.NSModalResponseOK)) return '';
    var paths = [];
    if (kind === 'save') {
        paths.push(ObjC.unwrap(panel.URL.path));
    } else {
        var urls = panel.URLs;
        for (var index = 0; index < Number(urls.count); index++) {
            paths.push(ObjC.unwrap(urls.objectAtIndex(index).path));
        }
    }
    return paths.join('\u0000') + '\u0000';
}`

func chooseNativePath(ctx context.Context, request pathRequest) ([]string, error) {
	if runtime.GOOS != "darwin" || os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return nil, errPathDialogUnavailable
	}
	directory, name, err := pathLocation(request.Initial)
	if err != nil {
		return nil, err
	}
	if request.Kind == "save" && request.Initial == "" {
		name = "backup.ta"
	}
	command := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-", request.Kind, "Telegram Archive · "+request.Title, directory, name, strconv.FormatBool(request.Multiple))
	command.Stdin = strings.NewReader(macPathDialogScript)
	output, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if failure, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w: %s", errPathDialogUnavailable, strings.TrimSpace(string(failure.Stderr)))
		}
		return nil, fmt.Errorf("%w: %v", errPathDialogUnavailable, err)
	}
	return decodeDialogPaths(string(output)), nil
}

func decodeDialogPaths(output string) []string {
	output = strings.TrimSuffix(output, "\n")
	if output == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(output, "\x00"), "\x00")
}
