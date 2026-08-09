package tests_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestWorkspacePersistPreservesForeignAuthors(t *testing.T) {
	dir := t.TempDir()

	alice := cog.NewWorkspace(dir)
	alice.ActiveAuthor = "Alice <alice@example.com>"

	h1 := common.PrimaryHash("observation-one")
	h2 := common.PrimaryHash("observation-two")

	alice.SetObservation(h1, cog.NewCOGObservation(
		common.NewObservationSchema("s0", "first answer"),
		[]*common.FileRange{newTestAnchor(dir, "pkg/a.go", 0, 0, 1, 1)},
	))
	alice.SetObservation(h2, cog.NewCOGObservation(
		common.NewObservationSchema("s1", "second answer"),
		nil,
	))

	aliceBytes, err := alice.Marshal()
	assert.NoError(t, err)

	bob := cog.NewWorkspace(dir)
	bob.ActiveAuthor = "Bob <bob@example.com>"
	assert.NoError(t, bob.Unmarshal(aliceBytes))

	// A pull-then-persist with no new observations must be byte-identical
	roundTrip, err := bob.Marshal()
	assert.NoError(t, err)
	assert.Equal(t, string(aliceBytes), string(roundTrip))

	h3 := common.PrimaryHash("observation-three")
	bob.SetObservation(h3, cog.NewCOGObservation(
		common.NewObservationSchema("s2", "third answer"),
		nil,
	))

	bobBytes, err := bob.Marshal()
	assert.NoError(t, err)

	// Alice's lines stay untouched; only Bob's new line is appended
	assert.True(t, bytes.HasPrefix(bobBytes, aliceBytes), "existing records must not be rewritten")

	lines := strings.Split(strings.TrimSpace(string(bobBytes)), "\n")
	if !assert.Len(t, lines, 3) {
		return
	}

	aliceAuthor := strings.Fields(lines[0])[0]
	bobAuthor := strings.Fields(lines[2])[0]
	assert.Equal(t, aliceAuthor, strings.Fields(lines[1])[0])
	assert.NotEqual(t, aliceAuthor, bobAuthor, "new records carry the current author")
}

func TestWorkspacePersistKeepsRecordsLandedOnDisk(t *testing.T) {
	dir := t.TempDir()

	session := cog.NewWorkspace(dir)
	session.ActiveAuthor = "Alice <alice@example.com>"

	hA := common.PrimaryHash("observation-a")
	session.SetObservation(hA, cog.NewCOGObservation(
		common.NewObservationSchema("s0", "session answer"),
		nil,
	))
	assert.NoError(t, session.Persist())

	// A teammate's record lands on disk mid-session, as after a git pull
	teammate := cog.NewWorkspace(dir)
	teammate.ActiveAuthor = "Bob <bob@example.com>"
	hK := common.PrimaryHash("observation-k")
	teammate.SetObservation(hK, cog.NewCOGObservation(
		common.NewObservationSchema("s0", "teammate answer"),
		nil,
	))
	teammateBytes, err := teammate.Marshal()
	assert.NoError(t, err)

	onDisk, err := os.ReadFile(session.FilePath())
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(session.FilePath(), append(onDisk, teammateBytes...), 0o644))

	hB := common.PrimaryHash("observation-b")
	session.SetObservation(hB, cog.NewCOGObservation(
		common.NewObservationSchema("s1", "second session answer"),
		nil,
	))
	assert.NoError(t, session.Persist())

	reloaded := cog.NewWorkspace(dir)
	persisted, err := os.ReadFile(session.FilePath())
	assert.NoError(t, err)
	assert.NoError(t, reloaded.Unmarshal(persisted))

	assert.Len(t, reloaded.ObservationCache, 3)
	assert.Equal(t, "teammate answer", reloaded.ObservationCache[hK].Answer.Answer)
	assert.NotEqual(t,
		reloaded.ObservationCache[hA].Author,
		reloaded.ObservationCache[hK].Author,
		"the teammate's record keeps the teammate's author stamp",
	)
}

func TestWorkspaceUnmarshalIgnoresDuplicateHash(t *testing.T) {
	dir := t.TempDir()
	h := common.PrimaryHash("shared-observation")

	first := cog.NewWorkspace(dir)
	first.ActiveAuthor = "Alice <alice@example.com>"
	first.SetObservation(h, cog.NewCOGObservation(
		common.NewObservationSchema("s0", "kept answer"),
		nil,
	))
	firstBytes, err := first.Marshal()
	assert.NoError(t, err)

	second := cog.NewWorkspace(dir)
	second.ActiveAuthor = "Bob <bob@example.com>"
	second.SetObservation(h, cog.NewCOGObservation(
		common.NewObservationSchema("s0", "ignored duplicate"),
		nil,
	))
	secondBytes, err := second.Marshal()
	assert.NoError(t, err)

	// Two authors committed the same observation separately and both lines
	// survived the merge; the first record wins, the duplicate is dropped
	merged := cog.NewWorkspace(dir)
	assert.NoError(t, merged.Unmarshal(append(firstBytes, secondBytes...)))

	assert.Len(t, merged.ObservationCache, 1)
	assert.Equal(t, "kept answer", merged.ObservationCache[h].Answer.Answer)

	remarshaled, err := merged.Marshal()
	assert.NoError(t, err)
	assert.Equal(t, string(firstBytes), string(remarshaled))
}
