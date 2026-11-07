# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.205"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.205/sshtermy_darwin_arm64"
      sha256 "5049c8d1b2b1dd7acb6b65256c00a8a420ed9ec4f4eac851688c2630ddb899ac"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.205/sshtermy_darwin_amd64"
      sha256 "65eaf969d34d6c2da4157083aa17987117890c8f1e1892979577cffa35d69904"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.205/sshtermy_linux_arm64"
      sha256 "ed237f795f9685493697601e03aa11c6d72d6dede5f1d48a96eaa0ec710f9b8f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.205/sshtermy_linux_amd64"
      sha256 "df0047e385d22d9b44e95c1290a21c2dbcf5c6a4575d988074ad3947d48f04a2"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
