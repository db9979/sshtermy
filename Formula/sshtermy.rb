# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.324"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.324/sshtermy_darwin_arm64"
      sha256 "8c9c25b61f4ef9e1e6db9aa455c6d2ba3246a19e4bb829ae9b9eb6c3ee87e8e6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.324/sshtermy_darwin_amd64"
      sha256 "a0b56f13625a6f6bdb4245463096699097ea45164dea58896ee27203c95b208f"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.324/sshtermy_linux_arm64"
      sha256 "57c193a76c9301f382134f257ccbcda22656ef78606f4d651cc02268b4f0097f"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.324/sshtermy_linux_amd64"
      sha256 "819e883ba43e48b9a2d1f05f0a9433e47e43687828f3d63ab8010396bc452fdb"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
