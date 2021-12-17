# Kubectl Real Name Diff

## What's this ?
If realname is specified in the label, diff between resources with the same label.
Other than that, it works just like normal kubectl diff.

This kubectl plugin assumes that you want to create ConfigMap, Secret with hash
suffix in Kustomize.
By specifying the real name without the hash suffix as a label, you can compare
the old and new resources regardless of the hash value.`)

## Example

```
# Diff resources included in the result of kustomize build
$ kustomize build ./example | kubectl realname-diff -f -

# Also you can use kubectl's built-in kustomize
$ kubectl realname-diff -k ./example`)
```
