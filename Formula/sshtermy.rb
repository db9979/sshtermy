# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.286"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.286/sshtermy_darwin_arm64"
      sha256 "ebb7a53f9308ffb0a3a412cb225c2e7a6ac44973b10bf539c8e6fd2f8217db2f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.286/sshtermy_darwin_amd64"
      sha256 "04ad02f43360ab5ac9c22006e52e3795571a0fe628503bb8f7296bcd3f86a987"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.286/sshtermy_linux_arm64"
      sha256 "e2676f9d743bedfed6cdb1e188ae5b1c681abc39b0eae88e8ea12a57cf153ec3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.286/sshtermy_linux_amd64"
      sha256 "3a84d6ce493bb728baf12ec5b60af9aa0571e85cc6d6f4f946677dfdfe0db6d0"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
