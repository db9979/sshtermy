# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.374"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.374/sshtermy_darwin_arm64"
      sha256 "890861ff46f2002c42e30ba93aefe0cd47ba4cf31588cc05c5a747fb4ba3ce48"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.374/sshtermy_darwin_amd64"
      sha256 "34e39a021ce64b3546a687a8bca5b13952423c879ebc680bfa78b9c486604fde"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.374/sshtermy_linux_arm64"
      sha256 "8fce3e757dfbd7042f8f3d84330dfedb1f79de7ea036cfcca33eac95aa7ebb6b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.374/sshtermy_linux_amd64"
      sha256 "8f339eee66209a92ab6b374cac01c2e80509d2e80b3a18c2963cada785009b5e"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
