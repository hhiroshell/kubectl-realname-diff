# Kubectl Real Name Diff
A kubectl plugin that diffs live and local resources ignoring Kustomize hash-suffixes.

## What's this ?
This is a variation of the kubectl diff command.

Normally, kubectl realname-diff works the same as kubectl diff, but if "real name" is
specified with the label, local and live resources with the same label will be compared.

This is especially beneficial if you have hash suffixed ConfigMaps or Secrets with the
Kustomize. In case of kubectl diff, local and live resources with hash suffixed name are
considered as irrelevant. So you will not be able to get any results comparing them.

However, with realname-diff, you can compare the resources with hash suffixed name by
specifying the comparison target with "real name" labels.

## Example

```
# Make sure you have already labeled the resources with "realname-diff/realname: [real name]"
# For a complete example, see https://github.com/hhiroshell/kubectl-realname-diff/tree/main/example 

# Diff resources included in the result of kustomize build
kustomize build ./example | kubectl realname-diff -f -

# Also you can use kubectl's built-in kustomize
kubectl realname-diff -k ./example`
```
