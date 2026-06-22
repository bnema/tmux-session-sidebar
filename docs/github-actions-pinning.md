# GitHub Actions pinning

GitHub Actions and reusable workflows are pinned to immutable commit SHAs so CI and release jobs do not execute mutable tag or branch contents.

## Updating pins

1. Pick the upstream release tag or branch update to review.
2. Resolve it to a commit SHA. For tags, prefer the peeled `^{}` SHA when present so annotated tag objects are not used as action pins:

   ```bash
   git ls-remote https://github.com/actions/checkout.git 'refs/tags/v6' 'refs/tags/v6^{}'
   git ls-remote https://github.com/bnema/gh-actions.git refs/heads/main
   ```

3. Review the upstream diff between the currently pinned SHA and the new SHA. Confirm the pinned action version supports every workflow feature in use, such as `go-version-file` inputs. For reusable workflows, review nested `uses:` entries too so mutable tag or branch refs are not reintroduced transitively.
4. Replace the SHA in `.github/workflows/*.yml` and update any pinned tool versions, such as `golangci-lint-action`'s `version` input.
5. Run the repository CI equivalent before merging:

   ```bash
   make ci
   ```
