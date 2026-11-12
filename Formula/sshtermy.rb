# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.380"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.380/sshtermy_darwin_arm64"
      sha256 "f24bd4f7ef9b7c1767abaac8d72bb9c2405ab52567c8ea56723ed76252013366"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.380/sshtermy_darwin_amd64"
      sha256 "aceacda5b22a57556927d4f9b99d73700001da0e34b4be81fde4792d053d8aed"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.380/sshtermy_linux_arm64"
      sha256 "f86d88dde238b6df44e265d6360d68dcae76ccad928a0597ff6012546d1768d0"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.380/sshtermy_linux_amd64"
      sha256 "d9d95bedc823ced08f092cfd9bb775d44254fe40639e76a98bec5b5ee84d5c51"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
