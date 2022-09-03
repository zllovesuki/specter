pipeline{
    agent {
        docker { image 'golang:1.19.0-buster' }
    }
    stages{
        stage("Install build-essential"){
            steps{
                sh "apt update -y && apt install -y build-essential"
            }
        }
        stage("Short Tests with Race Detector"){
            steps{
                sh "go test -v -short -race -timeout 120s ./..."
            }
        }
        stage("Run Integration Tests"){
            environment{
                GO_RUN_INTEGRATION = '1'
            }
            steps{
                sh "go test -v -timeout 60s ./integrations"
            }
        }
        stage("Run Full Test Suite"){
            steps{
                sh "make full_test TIMEOUT=600s"
            }
        }
    }
}