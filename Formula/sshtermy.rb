# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.215"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.215/sshtermy_darwin_arm64"
      sha256 "c10c15491510b1c13526eb4ca97a613a9553dd35638cee44d28e0cf8e4178032"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.215/sshtermy_darwin_amd64"
      sha256 "8963a5f82f3c6b3f2440bae043ad937cacb1d8d8548ad6a592c5dd9c643ba207"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.215/sshtermy_linux_arm64"
      sha256 "22902c1268260bf8fc4a5828dfa60114d5d1e2f1678b86244d05ac31c4caef77"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.215/sshtermy_linux_amd64"
      sha256 "8d1fca36254e1060810a7fad2625667cef06f9cd5f9a01db3f94a5a7cc71afd7"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
