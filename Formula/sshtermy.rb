# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  version "1.0.130"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.130/sshtermy_darwin_arm64"
      sha256 "4f64332fc1fbe08cf93a393d2aa760001825394d970dbb60b9022fb953f58da8"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.130/sshtermy_darwin_amd64"
      sha256 "7a15a64de083d23b8d76424b03672a0e914dca97f7adef9bc479b02e6b93b670"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.130/sshtermy_linux_arm64"
      sha256 "cbeca28b46092996393897b55435481190e6c25546565bc183be831057d2336f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.130/sshtermy_linux_amd64"
      sha256 "0b047addc1952f6be0bcb3de76d88a105b5b6a0b27e86af1eddcd68ccc962f15"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
