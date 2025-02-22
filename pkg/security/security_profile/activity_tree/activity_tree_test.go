// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package activity_tree

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestInsertFileEvent(t *testing.T) {
	pan := ProcessNode{
		Files: make(map[string]*FileNode),
	}
	pan.Process.FileEvent.PathnameStr = "/test/pan"
	stats := NewActivityTreeNodeStats()

	pathToInserts := []string{
		"/tmp/foo",
		"/tmp/bar",
		"/test/a/b/c/d/e/",
		"/hello",
		"/tmp/bar/test",
	}
	expectedDebugOuput := strings.TrimSpace(`
- process: /test/pan
  files:
    - hello
    - test
        - a
            - b
                - c
                    - d
                        - e
    - tmp
        - bar
            - test
        - foo
`)

	for _, path := range pathToInserts {
		event := &model.Event{
			Open: model.OpenEvent{
				File: model.FileEvent{
					IsPathnameStrResolved: true,
					PathnameStr:           path,
				},
			},
			FieldHandlers: &model.DefaultFieldHandlers{},
		}
		pan.InsertFileEvent(&event.Open.File, event, Unknown, stats, false, nil, nil)
	}

	var builder strings.Builder
	pan.debug(&builder, "")
	debugOutput := strings.TrimSpace(builder.String())

	assert.Equal(t, expectedDebugOuput, debugOutput)
}

func TestActivityTree_InsertExecEvent(t *testing.T) {
	for _, tt := range activityTreeInsertExecEventTestCases {
		t.Run(tt.name, func(t *testing.T) {
			node, newEntry, err := tt.tree.CreateProcessNode(tt.inputEvent.ProcessCacheEntry, nil, Runtime, false, nil)
			if tt.wantErr != nil {
				if !tt.wantErr(t, err, fmt.Sprintf("unexpected error: %v", err)) {
					return
				}
			} else if err != nil {
				t.Fatalf("an err was returned but none was expected: %v", err)
				return
			}

			var builder strings.Builder
			tt.tree.Debug(&builder)
			inputResult := strings.TrimSpace(builder.String())

			builder.Reset()
			tt.wantTree.Debug(&builder)
			wantedResult := strings.TrimSpace(builder.String())

			assert.Equalf(t, wantedResult, inputResult, "the generated tree didn't match the expected output")
			assert.Equalf(t, tt.wantNewEntry, newEntry, "invalid newEntry output")
			assert.Equalf(t, tt.wantNode.Process.FileEvent.PathnameStr, node.Process.FileEvent.PathnameStr, "the returned ProcessNode is invalid")
		})
	}
}

// activityTreeInsertTestValidator is a mock validator to test the activity tree insert feature
type activityTreeInsertTestValidator struct{}

func (a activityTreeInsertTestValidator) MatchesSelector(entry *model.ProcessCacheEntry) bool {
	return entry.ContainerID == "123"
}

func (a activityTreeInsertTestValidator) IsEventTypeValid(evtType model.EventType) bool {
	return true
}

func (a activityTreeInsertTestValidator) NewProcessNodeCallback(p *ProcessNode) {}

// newExecTestEventWithAncestors returns a new exec test event with a process cache entry populated with the input list.
// A final `systemd` node is appended.
func newExecTestEventWithAncestors(lineage []model.Process) *model.Event {
	// build the list of ancestors
	ancestor := new(model.ProcessCacheEntry)
	lineageDup := make([]model.Process, len(lineage))
	copy(lineageDup, lineage)

	// reverse lineageDup
	for i, j := 0, len(lineageDup)-1; i < j; i, j = i+1, j-1 {
		lineageDup[i], lineageDup[j] = lineageDup[j], lineageDup[i]
	}

	cursor := ancestor
	for _, p := range lineageDup[1:] {
		cursor.Process = p
		cursor.Ancestor = new(model.ProcessCacheEntry)
		cursor = cursor.Ancestor
	}

	// append systemd
	cursor.Process = model.Process{
		PIDContext: model.PIDContext{
			Pid: 1,
		},
		FileEvent: model.FileEvent{
			PathnameStr: "/bin/systemd",
			FileFields: model.FileFields{
				PathKey: model.PathKey{
					Inode: math.MaxUint64,
				},
			},
		},
	}

	evt := &model.Event{
		Type:             uint32(model.ExecEventType),
		FieldHandlers:    &model.DefaultFieldHandlers{},
		ContainerContext: &model.ContainerContext{},
		ProcessContext:   &model.ProcessContext{},
		Exec: model.ExecEvent{
			Process: &model.Process{},
		},
		ProcessCacheEntry: &model.ProcessCacheEntry{
			ProcessContext: model.ProcessContext{
				Process:  lineageDup[0],
				Ancestor: ancestor,
			},
		},
	}
	return evt
}

var activityTreeInsertExecEventTestCases = []struct {
	name         string
	tree         *ActivityTree
	inputEvent   *model.Event
	wantNewEntry bool
	wantErr      assert.ErrorAssertionFunc
	wantTree     *ActivityTree
	wantNode     *ProcessNode
}{
	// exec/1
	// ---------------
	//
	//     empty tree          +          systemd                 ==>>              /bin/bash
	//                                       |- /bin/bash                               |
	//                                       |- /bin/ls                              /bin/ls
	{
		name: "exec/1",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/2
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash
	//                                       |- /bin/bash                               |
	//                                       |- /bin/ls                              /bin/ls
	{
		name: "exec/2",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/3
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash ------------
	//          |                            |- /bin/bash                               |                |
	//      /bin/webserver                   |- /bin/ls                           /bin/webserver      /bin/ls
	//          |                                                                       |
	//       /bin/ls                                                                 /bin/ls
	{
		name: "exec/3",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
						{
							Process: model.Process{
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/4
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver                   |- /bin/ls                            /bin/webserver
	//          | (exec)                                                                | (exec)
	//       /bin/ls                                                                 /bin/ls
	{
		name: "exec/4",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/4_bis
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>              /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver---                |- /bin/ls                            /bin/webserver-----
	//          | (exec)    | (exec)                                                    | (exec)     | (exec)
	//       /bin/id     /bin/ls                                                     /bin/id      /bin/ls
	{
		name: "exec/4_bis",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/5
	// ---------------
	//
	//      /bin/webserver         +          systemd             ==>>           /bin/webserver
	//          | (exec)                       |- /bin/ls                              | (exec)
	//       /bin/ls                                                                 /bin/ls
	{
		name: "exec/5",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							IsExecChild: true,
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/6
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>               /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver1                  |- /bin/ls                           /bin/webserver1
	//          | (exec)                                                                | (exec)
	//     /bin/webserver2----------                                              /bin/webserver2---------
	//          | (exec)           |                                                    | (exec)         |
	//     /bin/webserver3      /bin/id                                           /bin/webserver3      /bin/id
	//          | (exec)                                                                | (exec)
	//     /bin/webserver4                                                        /bin/webserver4
	//          | (exec)                                                                | (exec)
	//       /bin/ls---------------                                                  /bin/ls--------------
	//          |                 |                                                     |                |
	//       /bin/wc           /bin/id                                               /bin/wc          /bin/id
	{
		name: "exec/6",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											IsExecChild: true,
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												ExitTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													IsExecChild: true,
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
														ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															IsExecChild: true,
															Process: model.Process{
																ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												ExitTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
														ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/7
	// ---------------
	//
	//      /bin/webserver1              +           systemd           ==>        /bin/webserver1
	//          | (exec)                              |- /bin/ls                        | (exec)
	//     /bin/webserver2----------                                              /bin/webserver2---------
	//          | (exec)           |                                                    | (exec)         |
	//     /bin/webserver3      /bin/id                                           /bin/webserver3     /bin/id
	//          | (exec)                                                                | (exec)
	//     /bin/webserver4                                                        /bin/webserver4
	//          | (exec)                                                                | (exec)
	//       /bin/ls---------------                                                  /bin/ls--------------
	//          |                 |                                                     |                |
	//       /bin/wc           /bin/id                                               /bin/wc          /bin/id
	{
		name: "exec/7",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver1",
						},
					},
					Children: []*ProcessNode{
						{
							IsExecChild: true,
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver2",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver3",
										},
									},
									Children: []*ProcessNode{
										{
											IsExecChild: true,
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver4",
												},
											},
											Children: []*ProcessNode{
												{
													IsExecChild: true,
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/ls",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/id",
																},
															},
														},
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/wc",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: false,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver1",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver2",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/id",
										},
									},
								},
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver3",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver4",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/ls",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/id",
																},
															},
														},
														{
															Process: model.Process{
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/wc",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/8
	// ---------------
	//
	//      /bin/bash          +          systemd                              ==>>              /bin/bash
	//          |                            |- /bin/bash                                             |
	//      /bin/ls                          |- /bin/webserver -> /bin/ls                       /bin/webserver
	//                                                                                                | (exec)
	//                                                                                             /bin/ls
	{
		name: "exec/8",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/9
	// ---------------
	//
	//      /bin/webserver      +          systemd                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver                           | (exec)
	//      /bin/ls                          |- /bin/ls                                         /bin/webserver
	//                                                                                                |
	//                                                                                             /bin/ls
	{
		name: "exec/9",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/10
	// ---------------
	//
	//      /bin/webserver      +          systemd                                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver -> /bin/apache                           | (exec)
	//      /bin/ls                          |- /bin/ls                                                         /bin/webserver------------
	//                                                                                                               | (exec)            |
	//                                                                                                          /bin/apache           /bin/ls
	//                                                                                                               |
	//                                                                                                            /bin/ls
	{
		name: "exec/10",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
								},
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/ls",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/11
	// ---------------
	//
	//      /bin/apache         +          systemd                                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver -> /bin/apache                           | (exec)
	//      /bin/ls                          |- /bin/ls                                                         /bin/webserver
	//                                                                                                               | (exec)
	//                                                                                                          /bin/apache
	//                                                                                                               |
	//                                                                                                            /bin/ls
	{
		name: "exec/11",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/apache",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/ls",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/12
	// ---------------
	//
	//      /bin/apache         +          systemd                                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver -> /bin/apache                           | (exec)
	//       /bin/ls                         |- /bin/wc -> /bin/id -> /bin/ls                                   /bin/webserver
	//          |                            |- /bin/date                                                            | (exec)
	//       /bin/date                       |- /bin/passwd -> /bin/bpftool -> /bin/du                           /bin/apache
	//          |                                                                                                     |
	//       /bin/du                                                                                               /bin/wc
	//                                                                                                               | (exec)
	//                                                                                                            /bin/id
	//                                                                                                               | (exec)
	//                                                                                                            /bin/ls
	//                                                                                                               |
	//                                                                                                            /bin/date
	//                                                                                                               |
	//                                                                                                           /bin/passwd
	//                                                                                                               | (exec)
	//                                                                                                           /bin/bpftool
	//                                                                                                               | (exec)
	//                                                                                                           /bin/du
	{
		name: "exec/12",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/apache",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/date",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/du",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/wc",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/id",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 5,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 26, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 6,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 26, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/date",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 7,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 27, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/passwd",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 8,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 28, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 29, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bpftool",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 9,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 29, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 30, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 10,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 30, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
														ExitTime: time.Date(2023, 06, 26, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/id",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																ExecTime: time.Date(2023, 06, 26, 1, 2, 3, 4, time.UTC),
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		ExecTime: time.Date(2023, 06, 27, 1, 2, 3, 4, time.UTC),
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/date",
																		},
																	},
																	Children: []*ProcessNode{
																		{
																			Process: model.Process{
																				ExecTime: time.Date(2023, 06, 28, 1, 2, 3, 4, time.UTC),
																				ExitTime: time.Date(2023, 06, 29, 1, 2, 3, 4, time.UTC),
																				FileEvent: model.FileEvent{
																					PathnameStr: "/bin/passwd",
																				},
																			},
																			Children: []*ProcessNode{
																				{
																					Process: model.Process{
																						ExecTime: time.Date(2023, 06, 29, 1, 2, 3, 4, time.UTC),
																						ExitTime: time.Date(2023, 06, 30, 1, 2, 3, 4, time.UTC),
																						FileEvent: model.FileEvent{
																							PathnameStr: "/bin/bpftool",
																						},
																					},
																					Children: []*ProcessNode{
																						{
																							Process: model.Process{
																								ExecTime: time.Date(2023, 06, 30, 1, 2, 3, 4, time.UTC),
																								FileEvent: model.FileEvent{
																									PathnameStr: "/bin/du",
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/13
	// ---------------
	//
	//      /bin/webserver      +          systemd                              ==>>              /bin/bash
	//          |                            |- /bin/bash -> /bin/webserver                           | (exec)
	//      /bin/ls                          |- /bin/wc                                         /bin/webserver
	//          | (exec)                                                                              |
	//       /bin/wc                                                                               /bin/ls
	//                                                                                                | (exec)
	//                                                                                             /bin/wc
	{
		name: "exec/13",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/ls",
								},
							},
							Children: []*ProcessNode{
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/wc",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/ls",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/14
	// ---------------
	//
	//      /bin/webserver      +          systemd                                     ==>>              /bin/bash
	//          | (exec)                     |- /bin/bash -> /bin/apache                                    | (exec)
	//      /bin/apache                      |- /bin/ls                                               /bin/webserver
	//          |                                                                                           | (exec)
	//       /bin/wc                                                                                     /bin/apache------
	//                                                                                                      |            |
	//                                                                                                   /bin/wc       /bin/ls
	{
		name: "exec/14",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							IsExecChild: true,
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/apache",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/wc",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
										},
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/ls",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/15
	// ---------------
	//
	//      /bin/webserver      +          systemd                                             ==>>              /bin/bash
	//          | (exec)                     |- /bin/bash -> /bin/du -> /bin/apache                                 | (exec)
	//      /bin/date                        |- /bin/ls                                                          /bin/du
	//          | (exec)                                                                                            | (exec)
	//      /bin/apache                                                                                       /bin/webserver
	//          |                                                                                                   | (exec)
	//       /bin/wc                                                                                            /bin/date
	//                                                                                                              | (exec)
	//                                                                                                          /bin/apache------
	//                                                                                                              |            |
	//                                                                                                          /bin/wc       /bin/ls
	{
		name: "exec/15",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/webserver",
						},
					},
					Children: []*ProcessNode{
						{
							IsExecChild: true,
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/date",
								},
							},
							Children: []*ProcessNode{
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/apache",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/wc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/du",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
				ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/apache",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
				ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						ExitTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 21, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/du",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 21, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
												ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/date",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/apache",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/wc",
																},
															},
														},
														{
															Process: model.Process{
																ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},

	// exec/16
	// ---------------
	//
	//      /bin/bash          +          systemd                 ==>>               /bin/bash
	//          |                            |- /bin/bash                               |
	//      /bin/webserver1                  |- /bin/webserver3                   /bin/webserver1
	//          | (exec)                     |- /bin/ls                                 | (exec)
	//     /bin/webserver2----------         |- /bin/date                         /bin/webserver2---------
	//          | (exec)           |                                                    | (exec)         |
	//     /bin/webserver3      /bin/id                                           /bin/webserver3      /bin/id
	//          |                                                                       |
	//     /bin/webserver4                                                        /bin/webserver4
	//          | (exec)                                                                | (exec)
	//       /bin/ls---------------                                                  /bin/ls----------------------------
	//          |                 |                                                     |                |             |
	//       /bin/wc           /bin/id                                               /bin/wc          /bin/id       /bin/date
	{
		name: "exec/16",
		tree: &ActivityTree{
			validator: activityTreeInsertTestValidator{},
			Stats:     NewActivityTreeNodeStats(),
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									IsExecChild: true,
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											IsExecChild: true,
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
														ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															IsExecChild: true,
															Process: model.Process{
																ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		inputEvent: newExecTestEventWithAncestors([]model.Process{
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/bash",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 1,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/webserver3",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 2,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/ls",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 3,
						},
					},
				},
			},
			{
				ContainerID: "123",
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/date",
					FileFields: model.FileFields{
						PathKey: model.PathKey{
							Inode: 4,
						},
					},
				},
			},
		}),
		wantNode: &ProcessNode{
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/bin/date",
				},
			},
		},
		wantNewEntry: true,
		wantTree: &ActivityTree{
			ProcessNodes: []*ProcessNode{
				{
					Process: model.Process{
						ExecTime: time.Date(2023, 06, 19, 1, 2, 3, 4, time.UTC),
						FileEvent: model.FileEvent{
							PathnameStr: "/bin/bash",
						},
					},
					Children: []*ProcessNode{
						{
							Process: model.Process{
								ExecTime: time.Date(2023, 06, 20, 1, 2, 3, 4, time.UTC),
								ExitTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
								FileEvent: model.FileEvent{
									PathnameStr: "/bin/webserver1",
								},
							},
							Children: []*ProcessNode{
								{
									Process: model.Process{
										ExecTime: time.Date(2023, 06, 22, 1, 2, 3, 4, time.UTC),
										ExitTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
										FileEvent: model.FileEvent{
											PathnameStr: "/bin/webserver2",
										},
									},
									Children: []*ProcessNode{
										{
											Process: model.Process{
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/id",
												},
											},
										},
										{
											Process: model.Process{
												ExecTime: time.Date(2023, 06, 23, 1, 2, 3, 4, time.UTC),
												FileEvent: model.FileEvent{
													PathnameStr: "/bin/webserver3",
												},
											},
											Children: []*ProcessNode{
												{
													Process: model.Process{
														ExecTime: time.Date(2023, 06, 24, 1, 2, 3, 4, time.UTC),
														ExitTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
														FileEvent: model.FileEvent{
															PathnameStr: "/bin/webserver4",
														},
													},
													Children: []*ProcessNode{
														{
															Process: model.Process{
																ExecTime: time.Date(2023, 06, 25, 1, 2, 3, 4, time.UTC),
																FileEvent: model.FileEvent{
																	PathnameStr: "/bin/ls",
																},
															},
															Children: []*ProcessNode{
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/id",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/wc",
																		},
																	},
																},
																{
																	Process: model.Process{
																		FileEvent: model.FileEvent{
																			PathnameStr: "/bin/date",
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}
