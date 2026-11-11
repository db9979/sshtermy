# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.348"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.348/sshtermy_darwin_arm64"
      sha256 "678ed945b3c6666ae6b096cf4f09dd010fab77e779ce177fb14d7b5365313b56"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.348/sshtermy_darwin_amd64"
      sha256 "a42ad4fa81895b411460c66a2a8b72d137b0e7000d7a9863de520293b719a753"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.348/sshtermy_linux_arm64"
      sha256 "6a4b92cd945166541cbdb39ddc3c4dacd9b05255dd8bb0f53cb5604a0fa1b1fe"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.348/sshtermy_linux_amd64"
      sha256 "094185112b9f7e4b7a3b675d98a893bb4d775535206f65cc25a7b09636f33d93"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
