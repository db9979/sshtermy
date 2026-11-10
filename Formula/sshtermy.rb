# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.330"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.330/sshtermy_darwin_arm64"
      sha256 "879ba0942393a2708df8db9a5c2b14f474dc2f37ccd6642d9fe941631f94a4ef"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.330/sshtermy_darwin_amd64"
      sha256 "e1bf99c95592526bedca5e8a6dcfdff4607e140622d375d7629b2f8624b532e1"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.330/sshtermy_linux_arm64"
      sha256 "fd17d2dfd844a7335949b8bbc05f079a8aa95e188cae2113e694a17bb4998a1a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.330/sshtermy_linux_amd64"
      sha256 "1ac04078ab61ec4134879d3bed0f662d756c936918d3d19108bd498d0829ca5d"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
