# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.334"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.334/sshtermy_darwin_arm64"
      sha256 "4a68cc957eca5360a1fe1b68b6de24165f5ac83704ac228eb6db95e95d52babb"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.334/sshtermy_darwin_amd64"
      sha256 "475f26f84f6c83d98ad6f22ddf8d719192d35509c010b67b8b943eb1697d7077"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.334/sshtermy_linux_arm64"
      sha256 "e3c547c32eb79c185b5e0d94b12e3cb2b765fc4504b74c82c9546cbcdb7a8364"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.334/sshtermy_linux_amd64"
      sha256 "f405408a004a5ebbaa78a73a449dd7ae58292c2bb2717d1c382c855b54200db5"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
