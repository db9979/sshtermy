# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.234"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.234/sshtermy_darwin_arm64"
      sha256 "9a3a463750d2463500838f8bfcc9fc70092e16b186be62b98a35c36bc4077bf5"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.234/sshtermy_darwin_amd64"
      sha256 "ea3364080547d01a3f39ca9ce09c7c481f6306d3a98bd00bf738b8aa4d94c291"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.234/sshtermy_linux_arm64"
      sha256 "a6bec79b78953324b718831e88b6f0124d66861f0bfc61a1d3c7743135b82661"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.234/sshtermy_linux_amd64"
      sha256 "c38ef3e7aec899ad411cc9aff264d83ec918940d8a212c81a4868a5694b130dd"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
