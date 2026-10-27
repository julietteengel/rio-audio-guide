# Two small repos, one per binary -- built and pushed from a dev machine
# (see deploy/terraform/README.md) rather than via a CI pipeline, since this
# is a one-off deployment, not a recurring release process.

resource "aws_ecr_repository" "api" {
  name                 = "rio-audio-guide/api"
  image_tag_mutability = "MUTABLE"
  force_delete         = true # fine to lose image history for this throwaway stack
}

resource "aws_ecr_repository" "worker" {
  name                 = "rio-audio-guide/worker"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
}
