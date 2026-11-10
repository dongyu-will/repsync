package workspace

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type ReviewAcknowledgement struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"createdAt"`
}

func reviewIssueKey(report ProjectReview, issue ReviewIssue) string {
	var context any
	switch {
	case strings.HasPrefix(issue.Kind, "stash_"):
		context = issue.Kind
	case issue.Kind == "untracked" || issue.Kind == "uncommitted" || issue.Kind == "conflicts":
		context = struct {
			Content string
			Files   []ReviewFile
		}{report.Snapshot.Fingerprint, report.Snapshot.Files}
	case slices.Contains([]string{"ahead", "behind", "upstream_unknown"}, issue.Kind):
		context = struct {
			Head, Branch, Upstream string
			Ahead, Behind          int
			Known                  bool
		}{report.Snapshot.Head, report.Snapshot.Branch, report.Snapshot.Upstream, report.Snapshot.Ahead, report.Snapshot.Behind, report.Snapshot.UpstreamKnown}
	case strings.HasPrefix(issue.Kind, "task_"):
		context = issue.Message
	default:
		context = struct {
			Message, Content, Config string
			Coverage                 []ReviewCoverageState
			Delivery                 []ReviewStage
			Resources                []ReviewResourceResult
		}{issue.Message, report.Snapshot.Fingerprint, report.ConfigVersion, report.Coverage, report.Delivery, report.Resources}
	}
	return gitDigest(struct {
		Project, Kind string
		Context       any
	}{report.Project.ID, issue.Kind, context})
}

func (s *Service) reviewAcknowledgements(report *ProjectReview) ([]ReviewAcknowledgement, error) {
	entries, err := readReviewRecordsLimit[ReviewAcknowledgement](filepath.Join(s.reviewDirectory(report.Project.Path), "acknowledgements"), 1000)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b ReviewAcknowledgement) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	latest := map[string]ReviewAcknowledgement{}
	for _, entry := range entries {
		if _, ok := latest[entry.Key]; !ok {
			latest[entry.Key] = entry
		}
	}
	pending := []ReviewIssue{}
	report.Acknowledged = []ReviewIssue{}
	for _, issue := range report.Issues {
		issue.Key = reviewIssueKey(*report, issue)
		if entry, ok := latest[issue.Key]; ok && (entry.State == "retained" || entry.State == "later") {
			copy := entry
			issue.Acknowledgement = &copy
			report.Acknowledged = append(report.Acknowledged, issue)
		} else {
			pending = append(pending, issue)
		}
	}
	report.Issues = pending
	return entries, nil
}

// SetReviewIssueState changes reminder handling, never the underlying Git or data state.
func (s *Service) SetReviewIssueState(path, version, key, state, reason string) (ReviewAcknowledgement, error) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	entry := ReviewAcknowledgement{}
	if !slices.Contains([]string{"retained", "later", "reopened"}, state) || len(reason) > 4096 || (state != "reopened" && strings.TrimSpace(reason) == "") {
		return entry, errors.New("请选择处理状态并填写保留或稍后处理的理由")
	}
	report, err := s.checkedReview(path, version)
	if err != nil {
		return entry, err
	}
	issues := append(slices.Clone(report.Issues), report.Acknowledged...)
	index := slices.IndexFunc(issues, func(i ReviewIssue) bool { return i.Key == key })
	if index < 0 {
		return entry, errors.New("该提醒已变化，请重新检查")
	}
	entry = ReviewAcknowledgement{ID: reviewRecordID(), Key: key, Kind: issues[index].Kind, State: state, Reason: reason, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	err = saveReviewRecord(filepath.Join(s.reviewDirectory(path), "acknowledgements", entry.ID+".json"), entry)
	return entry, err
}
