# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.290"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.290/sshtermy_darwin_arm64"
      sha256 "322110ac8a20aec036ca19e79e5a8d4b20933b52d6e9c7e84a7b9c5d41aafaab"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.290/sshtermy_darwin_amd64"
      sha256 "dde09f14cdeee0c27c0d73ffbece59cfa860a6d9bf076725f3cbbd3055cff807"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.290/sshtermy_linux_arm64"
      sha256 "6fb26283a0b01696179ea1eb10f2b3ddd77304266eceb8dd1767f6f1676ce5f3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.290/sshtermy_linux_amd64"
      sha256 "841dcac0714b438c59c93f04a9986663ab84028564accf1b1e3eaf213c565c45"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
