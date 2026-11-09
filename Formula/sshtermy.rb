# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.298"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.298/sshtermy_darwin_arm64"
      sha256 "da16ac015bba2ed3bb222387c5f08821a8c6489f7f8c01470bc6155fc3dbdd74"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.298/sshtermy_darwin_amd64"
      sha256 "7f0150a59c379e851a85de8bcb6b4950b7bdb55d678b77ef4100c7f4266c8aeb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.298/sshtermy_linux_arm64"
      sha256 "b0d15330fea4264f97313eae223c4b4c0f4279db42174ef009e42ee6bf0eebbc"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.298/sshtermy_linux_amd64"
      sha256 "a0331daad138060f1fadc3d1f379dea6eb42f3d8def9bc8e718adedbe5a18f22"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
