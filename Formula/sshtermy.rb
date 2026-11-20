# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.430"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.430/sshtermy_darwin_arm64"
      sha256 "ef014ac566e46c3796e07ac955a663fd9552be2e281fda651c50cca6a447e7fd"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.430/sshtermy_darwin_amd64"
      sha256 "74f49e76dca32933c415a89133f8a5411c6e4da829f278fffca1aa3ad7857007"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.430/sshtermy_linux_arm64"
      sha256 "d7ff14adf427f117111d516c40c578d9e4e64e273c1d54132618025c268eac1d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.430/sshtermy_linux_amd64"
      sha256 "f5bf2656507622ed5e45b86b754eb957352951e88225bcf60254825e3e700ea1"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
