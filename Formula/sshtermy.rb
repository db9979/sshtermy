# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.189"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.189/sshtermy_darwin_arm64"
      sha256 "f4e22efce0b9a98703a130858fd9728cc81bb5163855fbbc3a8e885d6b738fdb"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.189/sshtermy_darwin_amd64"
      sha256 "3a4471097046afc583095fcac826c224b82a2980765e7f1c17776599a797612e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.189/sshtermy_linux_arm64"
      sha256 "5fd892fe9314e20e81686cf74a4b323282881a1d54a5767345048d25139543d5"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.189/sshtermy_linux_amd64"
      sha256 "9e39bfa8a1d97f6afe54fa1523adb03402dc05feef53ad8a2b034cf7f5ac9395"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
