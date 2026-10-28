# Tracking test issues

Check capabilities for `issue_management` and `issue_history`. Use the public CLI:

```sh
malta --server URL issue save --file issue.json
malta --server URL issue get ISSUE_ID
malta --server URL issue history ISSUE_ID
malta --server URL issue list
```

Input uses issue_id, base_revision, title, type, description, software, hardware,
occurred_at (RFC3339), resolved (boolean), resolution, reporter and change_note.
Optional version_id/environment_id/run_id must exist on the target server and agree
with the linked run. `type` is free text, for example 功能, 性能 or 数据一致性.

Persist a stable issue ID and input before submission. Create with base_revision=0;
identical creation retries are accepted only while that issue is still revision 1.
On an uncertain response, get the issue/history before deciding whether to resubmit.
For updates use the latest revision as base_revision. Exit 6 means conflict: reload
and reconcile, never silently overwrite another reporter's work or allocate a duplicate ID.

Report observed evidence separately from hypotheses. Set resolved=true only with
verified resolution evidence; resolution is required. Reopen with resolved=false,
resolution="", and explain in change_note; historical resolutions are retained.
Uploading an issue does not authorize executing proposed fixes or additional tests.
Do not include credentials. Reporter is descriptive metadata, not verified identity.
The UI guide provides a downloadable template; full contract is in docs/issue-management.md.
